package storage

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestLocalStoragePutAndURL(t *testing.T) {
	dir := t.TempDir()
	svc, err := New(Config{LocalDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !svc.Local() {
		t.Fatal("expected local fallback")
	}
	if err := svc.Put(context.Background(), "properties/1/a.jpg", []byte("img"), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir + "/properties/1/a.jpg"); err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if url := svc.URL("properties/1/a.jpg"); url != "/media/properties/1/a.jpg" {
		t.Fatalf("URL = %q", url)
	}
}

func TestLocalStorageRejectsTraversal(t *testing.T) {
	svc, err := New(Config{LocalDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Put(context.Background(), "../../etc/passwd", []byte("x"), "text/plain"); err == nil {
		t.Fatal("expected traversal error")
	}
}

func TestCOSSignedURL(t *testing.T) {
	svc, err := New(Config{
		SecretID: "AKIDEXAMPLE", SecretKey: "secret", Bucket: "bucket-123", Region: "ap-shanghai",
	})
	if err != nil {
		t.Fatal(err)
	}
	url := svc.URL("properties/1/房源 图.jpg")
	if !strings.HasPrefix(url, "https://bucket-123.cos.ap-shanghai.myqcloud.com/") {
		t.Fatalf("unexpected host: %s", url)
	}
	for _, part := range []string{"q-sign-algorithm=sha1", "q-ak=AKIDEXAMPLE", "q-signature=", "q-header-list=host"} {
		if !strings.Contains(url, part) {
			t.Fatalf("signed URL missing %s: %s", part, url)
		}
	}
	if strings.Contains(url, " ") {
		t.Fatalf("key must be escaped: %s", url)
	}
}
