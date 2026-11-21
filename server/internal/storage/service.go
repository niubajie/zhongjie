// Package storage signs private object URLs and stores uploaded media.
// Production uses Tencent COS; development falls back to a local directory.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	SecretID  string
	SecretKey string
	Bucket    string
	Region    string
	Domain    string
	LocalDir  string
}

type Service struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config) (*Service, error) {
	if cfg.LocalDir == "" {
		cfg.LocalDir = "var/media"
	}
	s := &Service{cfg: cfg, client: &http.Client{Timeout: 30 * time.Second}}
	if !s.cosEnabled() {
		if err := os.MkdirAll(cfg.LocalDir, 0o750); err != nil {
			return nil, fmt.Errorf("create media dir: %w", err)
		}
	}
	return s, nil
}

func (s *Service) cosEnabled() bool {
	return s.cfg.SecretID != "" && s.cfg.SecretKey != "" && s.cfg.Bucket != "" && s.cfg.Region != ""
}

func (s *Service) Enabled() bool { return s.cosEnabled() }

func (s *Service) Local() bool { return !s.cosEnabled() }

// URL returns a short-lived read URL. Local mode serves through /media/:key.
func (s *Service) URL(key string) string {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if key == "" {
		return ""
	}
	if !s.cosEnabled() {
		return "/media/" + escapeKey(key)
	}
	return s.signedURL("get", key, 15*time.Minute)
}

func (s *Service) Put(ctx context.Context, key string, data []byte, contentType string) error {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if key == "" {
		return errors.New("empty object key")
	}
	if s.cosEnabled() {
		return s.putCOS(ctx, key, data, contentType)
	}
	path, err := s.localPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create media subdir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return fmt.Errorf("write media file: %w", err)
	}
	return nil
}

func (s *Service) LocalDir() string { return s.cfg.LocalDir }

// Get reads an object back so intake jobs can be re-recognized after a model
// provider is configured. Local mode reads the media directory directly.
func (s *Service) Get(ctx context.Context, key string) ([]byte, error) {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if key == "" {
		return nil, errors.New("empty object key")
	}
	if !s.cosEnabled() {
		path, err := s.localPath(key)
		if err != nil {
			return nil, err
		}
		return os.ReadFile(path)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.signedURL("get", key, 10*time.Minute), nil)
	if err != nil {
		return nil, fmt.Errorf("build media request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch media: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch media: status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 200<<20))
}

func (s *Service) localPath(key string) (string, error) {
	if strings.Contains(key, "..") || filepath.IsAbs(key) {
		return "", errors.New("invalid object key")
	}
	path := filepath.Join(s.cfg.LocalDir, filepath.FromSlash(key))
	base, err := filepath.Abs(s.cfg.LocalDir)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(abs, base+string(os.PathSeparator)) {
		return "", errors.New("invalid object key")
	}
	return abs, nil
}
