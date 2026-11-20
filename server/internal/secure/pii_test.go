package secure

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestPIIEncryptDecryptAndHash(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	p, err := NewPII(key, "test-hash-key")
	if err != nil {
		t.Fatalf("NewPII: %v", err)
	}

	enc, err := p.EncryptString("13800138000")
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	if bytes.Contains(enc, []byte("13800138000")) {
		t.Fatal("ciphertext contains plaintext")
	}
	got, err := p.DecryptString(enc)
	if err != nil {
		t.Fatalf("DecryptString: %v", err)
	}
	if got != "13800138000" {
		t.Fatalf("decrypted = %q", got)
	}
	if !bytes.Equal(p.HashPhone("138 0013 8000"), p.HashPhone("13800138000")) {
		t.Fatal("normalized hashes differ")
	}
}

func TestNormalizeAndMaskPhone(t *testing.T) {
	if got := NormalizePhone("+86 138-0013-8000"); got != "13800138000" {
		t.Fatalf("NormalizePhone = %q", got)
	}
	if got := MaskPhone("13800138000"); got != "138****8000" {
		t.Fatalf("MaskPhone = %q", got)
	}
}
