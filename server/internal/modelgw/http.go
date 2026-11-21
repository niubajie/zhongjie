package modelgw

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
)

const maxModelResponse = 8 << 20

func (g *Gateway) postJSON(ctx context.Context, provider Provider, path string, body any) (*http.Response, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, trimBase(provider.BaseURL)+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}
	return g.client.Do(req)
}

func readError(resp *http.Response, what string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("%s: upstream status %d: %s", what, resp.StatusCode, strings.TrimSpace(string(body)))
}

type chatChoice struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
	Usage   struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"usage"`
}

func (g *Gateway) chat(ctx context.Context, messages []Message) (string, error) {
	provider := g.cfg.LLM
	resp, err := g.postJSON(ctx, provider, "/chat/completions", map[string]any{
		"model":       provider.Model,
		"messages":    messages,
		"temperature": 0.2,
	})
	if err != nil {
		g.record(ctx, "llm", provider.Model, 0, 0, true)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		g.record(ctx, "llm", provider.Model, 0, 0, true)
		return "", readError(resp, "llm chat")
	}
	var out chatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxModelResponse)).Decode(&out); err != nil {
		g.record(ctx, "llm", provider.Model, 0, 0, true)
		return "", fmt.Errorf("decode llm response: %w", err)
	}
	if len(out.Choices) == 0 {
		g.record(ctx, "llm", provider.Model, 0, 0, true)
		return "", errors.New("llm returned an empty response")
	}
	g.record(ctx, "llm", provider.Model, out.Usage.PromptTokens, out.Usage.CompletionTokens, false)
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

func (g *Gateway) ocrText(ctx context.Context, image []byte, mime string) (string, error) {
	provider := g.cfg.OCR
	if mime == "" {
		mime = "image/jpeg"
	}
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(image)
	resp, err := g.postJSON(ctx, provider, "/chat/completions", map[string]any{
		"model": provider.Model,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "请提取这张房源图片中的全部文字，保留小区名、面积、租金、户型和联系电话，直接输出文字。"},
				{"type": "image_url", "image_url": map[string]string{"url": dataURL}},
			},
		}},
		"temperature": 0,
	})
	if err != nil {
		g.record(ctx, "ocr", provider.Model, int64(len(image)), 0, true)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		g.record(ctx, "ocr", provider.Model, int64(len(image)), 0, true)
		return "", readError(resp, "ocr")
	}
	var out chatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxModelResponse)).Decode(&out); err != nil {
		g.record(ctx, "ocr", provider.Model, int64(len(image)), 0, true)
		return "", fmt.Errorf("decode ocr response: %w", err)
	}
	if len(out.Choices) == 0 {
		g.record(ctx, "ocr", provider.Model, int64(len(image)), 0, true)
		return "", errors.New("ocr returned an empty response")
	}
	text := strings.TrimSpace(out.Choices[0].Message.Content)
	g.record(ctx, "ocr", provider.Model, int64(len(image)), int64(len(text)), false)
	return text, nil
}

func (g *Gateway) transcribe(ctx context.Context, audio []byte, mime string) (string, error) {
	provider := g.cfg.ASR
	if mime == "" {
		mime = "audio/wav"
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="audio"`)
	header.Set("Content-Type", mime)
	part, err := writer.CreatePart(header)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(audio); err != nil {
		return "", err
	}
	if err := writer.WriteField("model", provider.Model); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, trimBase(provider.BaseURL)+"/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if provider.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		g.record(ctx, "asr", provider.Model, int64(len(audio)), 0, true)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		g.record(ctx, "asr", provider.Model, int64(len(audio)), 0, true)
		return "", readError(resp, "asr")
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxModelResponse)).Decode(&out); err != nil {
		g.record(ctx, "asr", provider.Model, int64(len(audio)), 0, true)
		return "", fmt.Errorf("decode asr response: %w", err)
	}
	text := strings.TrimSpace(out.Text)
	g.record(ctx, "asr", provider.Model, int64(len(audio)), int64(len(text)), false)
	return text, nil
}

func (g *Gateway) synthesize(ctx context.Context, text string) ([]byte, string, error) {
	provider := g.cfg.TTS
	voice := provider.Voice
	if voice == "" {
		voice = "alloy"
	}
	resp, err := g.postJSON(ctx, provider, "/audio/speech", map[string]any{
		"model":           provider.Model,
		"voice":           voice,
		"input":           text,
		"response_format": "mp3",
	})
	if err != nil {
		g.record(ctx, "tts", provider.Model, int64(len(text)), 0, true)
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		g.record(ctx, "tts", provider.Model, int64(len(text)), 0, true)
		return nil, "", readError(resp, "tts")
	}
	audio, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		g.record(ctx, "tts", provider.Model, int64(len(text)), 0, true)
		return nil, "", err
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = "audio/mpeg"
	}
	g.record(ctx, "tts", provider.Model, int64(len(text)), int64(len(audio)), false)
	return audio, mime, nil
}
