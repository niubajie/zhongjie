// Package modelgw is the single entry point for OCR, ASR, TTS and LLM calls.
// Business code must never call a vendor SDK directly: swapping providers and
// recording cost should only require changing configuration.
package modelgw

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotConfigured = errors.New("model gateway not configured")

type Provider struct {
	BaseURL string
	APIKey  string
	Model   string
	Voice   string
}

func (p Provider) configured() bool {
	return strings.TrimSpace(p.BaseURL) != ""
}

type Config struct {
	OCR Provider
	ASR Provider
	TTS Provider
	LLM Provider
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Status struct {
	OCR bool `json:"ocr"`
	ASR bool `json:"asr"`
	TTS bool `json:"tts"`
	LLM bool `json:"llm"`
}

type Gateway struct {
	client *http.Client
	pool   *pgxpool.Pool
	log    *slog.Logger
	cfg    Config
}

func New(cfg Config, pool *pgxpool.Pool, log *slog.Logger) *Gateway {
	if log == nil {
		log = slog.Default()
	}
	return &Gateway{
		client: &http.Client{Timeout: 90 * time.Second},
		pool:   pool,
		log:    log,
		cfg:    cfg,
	}
}

func (g *Gateway) Status() Status {
	return Status{
		OCR: g.cfg.OCR.configured(),
		ASR: g.cfg.ASR.configured(),
		TTS: g.cfg.TTS.configured(),
		LLM: g.cfg.LLM.configured(),
	}
}

func (g *Gateway) OCRText(ctx context.Context, image []byte, mime string) (string, error) {
	if !g.cfg.OCR.configured() {
		return "", ErrNotConfigured
	}
	return g.ocrText(ctx, image, mime)
}

func (g *Gateway) Transcribe(ctx context.Context, audio []byte, mime string) (string, error) {
	if !g.cfg.ASR.configured() {
		return "", ErrNotConfigured
	}
	return g.transcribe(ctx, audio, mime)
}

func (g *Gateway) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	if !g.cfg.TTS.configured() {
		return nil, "", ErrNotConfigured
	}
	return g.synthesize(ctx, text)
}

func (g *Gateway) Chat(ctx context.Context, messages []Message) (string, error) {
	if !g.cfg.LLM.configured() {
		return "", ErrNotConfigured
	}
	return g.chat(ctx, messages)
}

func trimBase(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}
