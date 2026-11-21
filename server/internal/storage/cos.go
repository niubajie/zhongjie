package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Service) cosHost() string {
	return s.cfg.Bucket + ".cos." + s.cfg.Region + ".myqcloud.com"
}

func hmacSHA1(key, value string) []byte {
	h := hmac.New(sha1.New, []byte(key))
	h.Write([]byte(value))
	return h.Sum(nil)
}

func sha1Hex(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func escapeKey(key string) string {
	parts := strings.Split(key, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func (s *Service) signedURL(method, key string, ttl time.Duration) string {
	host := s.cosHost()
	path := "/" + escapeKey(key)
	now := time.Now().Unix()
	keyTime := fmt.Sprintf("%d;%d", now, now+int64(ttl.Seconds()))

	signKey := hmacSHA1(s.cfg.SecretKey, keyTime)
	httpString := strings.ToLower(method) + "\n" + path + "\n\nhost=" + host + "\n"
	stringToSign := "sha1\n" + keyTime + "\n" + sha1Hex(httpString) + "\n"
	signature := hex.EncodeToString(hmacSHA1(string(signKey), stringToSign))

	query := url.Values{}
	query.Set("q-sign-algorithm", "sha1")
	query.Set("q-ak", s.cfg.SecretID)
	query.Set("q-sign-time", keyTime)
	query.Set("q-key-time", keyTime)
	query.Set("q-header-list", "host")
	query.Set("q-url-param-list", "")
	query.Set("q-signature", signature)
	return "https://" + host + path + "?" + query.Encode()
}

func (s *Service) putCOS(ctx context.Context, key string, data []byte, contentType string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.signedURL("put", key, 10*time.Minute), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build cos put request: %w", err)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("cos put: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("cos put status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
