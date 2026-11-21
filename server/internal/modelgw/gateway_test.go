package modelgw

import (
	"context"
	"errors"
	"testing"
)

func TestGatewayNotConfigured(t *testing.T) {
	gw := New(Config{}, nil, nil)
	ctx := context.Background()

	if status := gw.Status(); status.OCR || status.ASR || status.TTS || status.LLM {
		t.Fatalf("empty config must report not configured: %+v", status)
	}
	if _, err := gw.OCRText(ctx, []byte("x"), "image/jpeg"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("OCRText error = %v, want ErrNotConfigured", err)
	}
	if _, err := gw.Transcribe(ctx, []byte("x"), "audio/wav"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Transcribe error = %v, want ErrNotConfigured", err)
	}
	if _, _, err := gw.Synthesize(ctx, "你好"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Synthesize error = %v, want ErrNotConfigured", err)
	}
	if _, err := gw.Chat(ctx, []Message{{Role: "user", Content: "hi"}}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Chat error = %v, want ErrNotConfigured", err)
	}
}

func TestGatewayStatusPartial(t *testing.T) {
	gw := New(Config{ASR: Provider{BaseURL: "http://asr.local", Model: "w"}}, nil, nil)
	status := gw.Status()
	if !status.ASR || status.TTS || status.LLM || status.OCR {
		t.Fatalf("unexpected status: %+v", status)
	}
}
