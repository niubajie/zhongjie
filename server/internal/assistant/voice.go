package assistant

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"housing/internal/httpx"
	"housing/internal/modelgw"
)

const (
	voiceSampleRate   = 16000
	voiceBytesPerMs   = voiceSampleRate * 2 / 1000
	voiceFrameMs      = 20
	voiceSilenceMs    = 600
	voicePreRollMs    = 300
	voiceMaxTurnMs    = 30000
	voiceRMSThreshold = 420
)

type voiceWriter struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func (w *voiceWriter) send(v any) {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	_ = w.conn.WriteJSON(v)
}

func (w *voiceWriter) close() {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	_ = w.conn.Close()
}

type voiceEvent struct {
	Type   string         `json:"type"`
	Text   string         `json:"text,omitempty"`
	Seq    int            `json:"seq,omitempty"`
	URL    string         `json:"url,omitempty"`
	Hint   string         `json:"hint,omitempty"`
	Slots  map[string]any `json:"slots,omitempty"`
	Status string         `json:"status,omitempty"`
}

func (h *Handler) RegisterVoice(r *gin.RouterGroup) {
	r.GET("/assistant/voice/status", h.voiceStatus)
	r.POST("/assistant/voice/turn", h.voiceTurn)
	r.GET("/assistant/voice", h.voiceSocket)
}

func (h *Handler) voiceStatus(c *gin.Context) {
	httpx.OK(c, h.svc.VoiceStatus())
}

// voiceTrun is a one-shot fallback for clients that cannot keep a socket open.
func (h *Handler) voiceTurn(c *gin.Context) {
	sessionID := strings.TrimSpace(c.Query("sessionId"))
	if sessionID == "" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "缺少 sessionId")
		return
	}
	var audio []byte
	var mime string
	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		file, header, err := c.Request.FormFile("audio")
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "没有收到音频")
			return
		}
		defer file.Close()
		audio, err = io.ReadAll(io.LimitReader(file, 12<<20))
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "读取音频失败")
			return
		}
		mime = header.Header.Get("Content-Type")
		if mime == "" {
			mime = "audio/wav"
		}
	} else {
		raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 12<<20))
		if err != nil || len(raw) == 0 {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "没有收到音频")
			return
		}
		audio = raw
		mime = c.ContentType()
		if mime == "" {
			mime = "audio/wav"
		}
	}
	if len(audio) == 0 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "没有收到音频")
		return
	}
	text, err := h.svc.Transcribe(c.Request.Context(), audio, mime)
	if err != nil {
		h.voiceError(c, err)
		return
	}
	reply, err := h.svc.Message(c.Request.Context(), MessageInput{SessionID: sessionID, Text: text})
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, "会话不存在或已过期")
			return
		}
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		return
	}
	sentences := splitSentences(reply.Reply, 6)
	audioURLs := make([]string, 0, len(sentences))
	for i, sentence := range sentences {
		url, err := h.svc.Speak(c.Request.Context(), i+1, sentence)
		if err != nil {
			h.voiceError(c, err)
			return
		}
		audioURLs = append(audioURLs, url)
	}
	httpx.OK(c, gin.H{
		"transcript": text,
		"reply":      reply.Reply,
		"slots":      reply.Slots,
		"ready":      reply.Ready,
		"action":     reply.Action,
		"audioUrls":  audioURLs,
	})
}

func (h *Handler) voiceError(c *gin.Context, err error) {
	if errors.Is(err, modelgw.ErrNotConfigured) {
		httpx.Fail(c, http.StatusServiceUnavailable, httpx.CodeInternal, "语音能力未配置，请先配置模型网关")
		return
	}
	httpx.Fail(c, http.StatusBadGateway, httpx.CodeInternal, "语音服务暂时不可用")
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin:     func(*http.Request) bool { return true },
}

func (h *Handler) voiceSocket(c *gin.Context) {
	sessionID := strings.TrimSpace(c.Query("sessionId"))
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": httpx.CodeInvalidArgument, "message": "缺少 sessionId"})
		return
	}
	if _, err := h.svc.repo.GetSession(c.Request.Context(), sessionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": httpx.CodeInvalidArgument, "message": "会话不存在或已过期"})
		return
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	writer := &voiceWriter{conn: conn}
	defer writer.close()

	if !h.svc.VoiceStatus().ASR {
		writer.send(voiceEvent{Type: "error", Status: "not_configured", Hint: "语音识别未配置，可改用文字输入"})
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var turnMu sync.Mutex
	var turnCancel context.CancelFunc
	startTurn := func(audio []byte) {
		turnCtx, c := context.WithCancel(ctx)
		turnMu.Lock()
		turnCancel = c
		turnMu.Unlock()
		go func() {
			defer c()
			h.runTurn(turnCtx, sessionID, audio, writer)
		}()
	}
	interrupt := func() {
		turnMu.Lock()
		if turnCancel != nil {
			turnCancel()
			turnCancel = nil
		}
		turnMu.Unlock()
		writer.send(voiceEvent{Type: "interrupted"})
	}

	vad := &vadState{}
	var buf []byte
	for {
		typ, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if typ == websocket.TextMessage {
			var msg struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(data, &msg) == nil && msg.Type == "interrupt" {
				interrupt()
			}
			continue
		}
		for off := 0; off+voiceFrameMs*voiceBytesPerMs <= len(data); off += voiceFrameMs * voiceBytesPerMs {
			frame := data[off : off+voiceFrameMs*voiceBytesPerMs]
			ended, started := vad.feed(frame)
			if started {
				interrupt()
				writer.send(voiceEvent{Type: "vad", Status: "speech"})
			}
			buf = append(buf, frame...)
			if !vad.speech && len(buf) > voicePreRollMs*voiceBytesPerMs {
				buf = buf[len(buf)-voicePreRollMs*voiceBytesPerMs:]
			}
			if len(buf) > voiceMaxTurnMs*voiceBytesPerMs {
				ended = true
			}
			if ended {
				audio := pcmToWAV(buf)
				buf = nil
				vad.reset()
				writer.send(voiceEvent{Type: "vad", Status: "silence"})
				startTurn(audio)
			}
		}
	}
}

func (h *Handler) runTurn(ctx context.Context, sessionID string, audio []byte, writer *voiceWriter) {
	writer.send(voiceEvent{Type: "processing"})
	text, err := h.svc.Transcribe(ctx, audio, "audio/wav")
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		writer.send(voiceEvent{Type: "error", Status: "asr_failed", Hint: "没有听清，请再说一次"})
		return
	}
	if ctx.Err() != nil {
		return
	}
	writer.send(voiceEvent{Type: "asr", Text: text})
	reply, err := h.svc.Message(ctx, MessageInput{SessionID: sessionID, Text: text})
	if err != nil {
		writer.send(voiceEvent{Type: "error", Status: "assistant_failed", Hint: "小亿暂时没反应过来"})
		return
	}
	writer.send(voiceEvent{Type: "reply", Text: reply.Reply, Slots: reply.Slots})
	for i, sentence := range splitSentences(reply.Reply, 6) {
		if ctx.Err() != nil {
			return
		}
		url, err := h.svc.Speak(ctx, i+1, sentence)
		if err != nil {
			if ctx.Err() == nil {
				writer.send(voiceEvent{Type: "error", Status: "tts_failed", Hint: "语音播报不可用，请看文字"})
			}
			return
		}
		writer.send(voiceEvent{Type: "audio", Seq: i + 1, URL: url, Text: sentence})
	}
	writer.send(voiceEvent{Type: "turn_end"})
}

type vadState struct {
	speech    bool
	speechMs  int
	silenceMs int
}

func (v *vadState) reset() {
	v.speech = false
	v.speechMs = 0
	v.silenceMs = 0
}

func (v *vadState) feed(frame []byte) (ended, started bool) {
	if len(frame) < 2 {
		return false, false
	}
	var sum float64
	samples := len(frame) / 2
	for i := 0; i < samples; i++ {
		s := int16(binary.LittleEndian.Uint16(frame[i*2:]))
		sum += float64(s) * float64(s)
	}
	rms := math.Sqrt(sum / float64(samples))
	if rms >= voiceRMSThreshold {
		v.speechMs += voiceFrameMs
		v.silenceMs = 0
		if !v.speech && v.speechMs >= 100 {
			v.speech = true
			return false, true
		}
		return false, false
	}
	if v.speech {
		v.silenceMs += voiceFrameMs
		if v.silenceMs >= voiceSilenceMs {
			return true, false
		}
	} else {
		v.speechMs = 0
	}
	return false, false
}

func pcmToWAV(pcm []byte) []byte {
	var buf bytes.Buffer
	dataLen := uint32(len(pcm))
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+len(pcm)))
	buf.WriteString("WAVEfmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(voiceSampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(voiceSampleRate*2))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, dataLen)
	buf.Write(pcm)
	return buf.Bytes()
}

func splitSentences(text string, limit int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	cut := func(r rune) bool {
		return r == '。' || r == '！' || r == '？' || r == '；' || r == '!' || r == '?' || r == ';' || r == '\n'
	}
	parts := strings.FieldsFunc(text, cut)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part+"。")
		if len(out) >= limit {
			break
		}
	}
	if len(out) == 0 {
		out = append(out, text)
	}
	return out
}
