package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// PII provides field-level encryption and deterministic hashes for phone
// numbers. Ciphertext is non-deterministic (AES-GCM); the HMAC hash is used
// only for equality lookups.
type PII struct {
	aead    cipher.AEAD
	hashKey []byte
}

func NewPII(encKeyBase64, hashKey string) (*PII, error) {
	if strings.TrimSpace(hashKey) == "" {
		return nil, errors.New("empty PII hash key")
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encKeyBase64))
	if err != nil {
		return nil, fmt.Errorf("decode PII_ENC_KEY: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("PII_ENC_KEY must decode to 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &PII{aead: aead, hashKey: []byte(hashKey)}, nil
}

func (p *PII) EncryptString(plain string) ([]byte, error) {
	if plain == "" {
		return nil, nil
	}
	nonce := make([]byte, p.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return p.aead.Seal(nonce, nonce, []byte(plain), nil), nil
}

func (p *PII) DecryptString(ciphertext []byte) (string, error) {
	if len(ciphertext) == 0 {
		return "", nil
	}
	ns := p.aead.NonceSize()
	if len(ciphertext) < ns {
		return "", errors.New("ciphertext too short")
	}
	plain, err := p.aead.Open(nil, ciphertext[:ns], ciphertext[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// Hash normalizes the value before HMAC so "138 0013 8000" and
// "13800138000" map to the same key. Use for phone / WeChat ID.
func (p *PII) Hash(value string) []byte {
	mac := hmac.New(sha256.New, p.hashKey)
	mac.Write([]byte(strings.ToLower(strings.TrimSpace(value))))
	return mac.Sum(nil)
}

// HashPhone normalizes punctuation/space before hashing so the same number
// always maps to one lookup key.
func (p *PII) HashPhone(raw string) []byte {
	return p.Hash(NormalizePhone(raw))
}

func NormalizePhone(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	digits := b.String()
	switch {
	case len(digits) == 13 && strings.HasPrefix(digits, "86"):
		return digits[2:]
	case len(digits) == 11 && strings.HasPrefix(digits, "1"):
		return digits
	default:
		return digits
	}
}

func MaskPhone(phone string) string {
	p := NormalizePhone(phone)
	if len(p) != 11 {
		if len(p) <= 4 {
			return "****"
		}
		return p[:2] + "****" + p[len(p)-2:]
	}
	return p[:3] + "****" + p[7:]
}
