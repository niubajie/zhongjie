package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"housing/internal/notify"
	"housing/internal/wechat"
)

// Config is the runtime configuration. Secrets must come from environment
// variables in production; the dev defaults only exist so the project can run
// locally without a secret manager.
type Config struct {
	Env                       string
	HTTPAddr                  string
	DatabaseURL               string
	RedisURL                  string
	JWTSecret                 string
	JWTAccessTTL              time.Duration
	AdminJWTSecret            string
	AdminJWTAccessTTL         time.Duration
	WeChatAppID               string
	WeChatSecret              string
	WeChatAPIBase             string
	WeChatSubscribeTemplateID string
	WeChatSubscribePage       string
	NotifyMaxAttempts         int
	PIIEncKey                 string
	PIIHashKey                string
	DevLoginEnabled           bool
	LogLevel                  string
	AllowedAdminOrigin        string

	// Model gateway (OCR/ASR/TTS/LLM). Empty base URL means not configured.
	ModelOCRBaseURL string
	ModelOCRApiKey  string
	ModelOCRModel   string
	ModelASRBaseURL string
	ModelASRApiKey  string
	ModelASRModel   string
	ModelTTSBaseURL string
	ModelTTSApiKey  string
	ModelTTSModel   string
	ModelTTSVoice   string
	ModelLLMBaseURL string
	ModelLLMApiKey  string
	ModelLLMModel   string

	// Object storage. Without COS credentials the server falls back to a local
	// directory for development.
	COSSecretID  string
	COSSecretKey string
	COSBucket    string
	COSRegion    string
	COSDomain    string
	MediaDir     string

	// Map of subscribe-message template fields, e.g.
	// "thing1=reminder,thing2=title,thing3=location".
	NotifyFields string

	// AdminNotifyUserID 接收获客提醒微信模板的用户 ID；0 表示不开启微信外发。
	AdminNotifyUserID int64
}

func Load() (Config, error) {
	loadDotEnv(".env")
	env := strings.ToLower(getenv("APP_ENV", "dev"))
	if env != "dev" && env != "test" && env != "prod" {
		return Config{}, fmt.Errorf("APP_ENV must be dev|test|prod, got %q", env)
	}

	cfg := Config{
		Env:                       env,
		HTTPAddr:                  getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:               getenv("DATABASE_URL", "postgres://housing:housing@localhost:56000/housing?sslmode=disable"),
		RedisURL:                  getenv("REDIS_URL", "redis://localhost:56379/0"),
		JWTSecret:                 os.Getenv("JWT_SECRET"),
		AdminJWTSecret:            os.Getenv("ADMIN_JWT_SECRET"),
		WeChatAppID:               os.Getenv("WECHAT_APPID"),
		WeChatSecret:              os.Getenv("WECHAT_SECRET"),
		WeChatAPIBase:             getenv("WECHAT_API_BASE", wechat.DefaultAPIBase),
		WeChatSubscribeTemplateID: getenv("WECHAT_SUBSCRIBE_TEMPLATE_ID", ""),
		WeChatSubscribePage:       getenv("WECHAT_SUBSCRIBE_PAGE", "pages/map/index"),
		NotifyMaxAttempts:         getenvInt("NOTIFY_MAX_ATTEMPTS", notify.DefaultMaxAttempts),
		PIIEncKey:                 os.Getenv("PII_ENC_KEY"),
		PIIHashKey:                os.Getenv("PII_HASH_KEY"),
		LogLevel:                  getenv("LOG_LEVEL", "info"),
		AllowedAdminOrigin:        getenv("ADMIN_ORIGIN", "http://localhost:5173,http://127.0.0.1:5173"),

		ModelOCRBaseURL: os.Getenv("MODELGW_OCR_BASE_URL"),
		ModelOCRApiKey:  os.Getenv("MODELGW_OCR_API_KEY"),
		ModelOCRModel:   getenv("MODELGW_OCR_MODEL", "ocr"),
		ModelASRBaseURL: os.Getenv("MODELGW_ASR_BASE_URL"),
		ModelASRApiKey:  os.Getenv("MODELGW_ASR_API_KEY"),
		ModelASRModel:   getenv("MODELGW_ASR_MODEL", "whisper-1"),
		ModelTTSBaseURL: os.Getenv("MODELGW_TTS_BASE_URL"),
		ModelTTSApiKey:  os.Getenv("MODELGW_TTS_API_KEY"),
		ModelTTSModel:   getenv("MODELGW_TTS_MODEL", "tts-1"),
		ModelTTSVoice:   getenv("MODELGW_TTS_VOICE", "alloy"),
		ModelLLMBaseURL: os.Getenv("MODELGW_LLM_BASE_URL"),
		ModelLLMApiKey:  os.Getenv("MODELGW_LLM_API_KEY"),
		ModelLLMModel:   getenv("MODELGW_LLM_MODEL", "gpt-4o-mini"),

		COSSecretID:  os.Getenv("COS_SECRET_ID"),
		COSSecretKey: os.Getenv("COS_SECRET_KEY"),
		COSBucket:    os.Getenv("COS_BUCKET"),
		COSRegion:    os.Getenv("COS_REGION"),
		COSDomain:    os.Getenv("COS_DOMAIN"),
		MediaDir:     getenv("MEDIA_DIR", "var/media"),
		NotifyFields: os.Getenv("WECHAT_SUBSCRIBE_FIELDS"),
	}

	ttl, err := time.ParseDuration(getenv("JWT_ACCESS_TTL", defaultAccessTTL(env)))
	if err != nil {
		return Config{}, fmt.Errorf("JWT_ACCESS_TTL: %w", err)
	}
	cfg.JWTAccessTTL = ttl

	adminTTL, err := time.ParseDuration(getenv("ADMIN_JWT_ACCESS_TTL", "8h"))
	if err != nil {
		return Config{}, fmt.Errorf("ADMIN_JWT_ACCESS_TTL: %w", err)
	}
	cfg.AdminJWTAccessTTL = adminTTL

	adminNotifyUserID, err := strconv.ParseInt(getenv("ADMIN_NOTIFY_USER_ID", "0"), 10, 64)
	if err != nil {
		return Config{}, fmt.Errorf("ADMIN_NOTIFY_USER_ID: %w", err)
	}
	cfg.AdminNotifyUserID = adminNotifyUserID

	devLogin, err := strconv.ParseBool(getenv("DEV_LOGIN_ENABLED", boolString(env == "dev")))
	if err != nil {
		return Config{}, fmt.Errorf("DEV_LOGIN_ENABLED: %w", err)
	}
	cfg.DevLoginEnabled = devLogin

	if env == "prod" {
		if len(cfg.JWTSecret) < 32 {
			return Config{}, errors.New("JWT_SECRET must be at least 32 chars in prod")
		}
		if len(cfg.AdminJWTSecret) < 32 {
			return Config{}, errors.New("ADMIN_JWT_SECRET must be at least 32 chars in prod")
		}
		if cfg.WeChatAppID == "" || cfg.WeChatSecret == "" {
			return Config{}, errors.New("WECHAT_APPID and WECHAT_SECRET are required in prod")
		}
		if cfg.PIIEncKey == "" || cfg.PIIHashKey == "" {
			return Config{}, errors.New("PII_ENC_KEY and PII_HASH_KEY are required in prod")
		}
		if cfg.DevLoginEnabled {
			return Config{}, errors.New("DEV_LOGIN_ENABLED must be false in prod")
		}
	}

	if cfg.JWTSecret == "" {
		cfg.JWTSecret = "dev-only-jwt-secret-change-before-prod-123456"
	}
	if cfg.AdminJWTSecret == "" {
		cfg.AdminJWTSecret = "dev-only-admin-jwt-secret-change-before-prod-123"
	}
	if cfg.PIIEncKey == "" {
		// base64 of "0123456789abcdef0123456789abcdef" (32 bytes)
		cfg.PIIEncKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	}
	if cfg.PIIHashKey == "" {
		cfg.PIIHashKey = "dev-only-pii-hash-key-change-before-prod"
	}

	return cfg, nil
}

func defaultAccessTTL(env string) string {
	if env == "prod" {
		return "15m"
	}
	return "24h"
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// loadDotEnv is a tiny, dependency-free .env reader for local development.
// Existing environment variables always win; production should inject secrets
// from the platform, not from a file in the working directory.
func loadDotEnv(path string) {
	body, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, value)
	}
}
