package intake

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"testing"

	"housing/internal/secure"
)

func TestParseTextExtractsListingFields(t *testing.T) {
	text := "小区：幸福里\n面积：89 平\n月租：4500 元\n户型：3室2厅\n联系 13800138000"
	out := parseText(text)
	if out.CommunityName != "幸福里" {
		t.Fatalf("CommunityName = %q, want 幸福里", out.CommunityName)
	}
	if out.AreaSqm != 89 {
		t.Fatalf("AreaSqm = %v, want 89", out.AreaSqm)
	}
	if out.RentYuan != 4500 {
		t.Fatalf("RentYuan = %v, want 4500", out.RentYuan)
	}
	if out.Layout != "3室2厅" {
		t.Fatalf("Layout = %q, want 3室2厅", out.Layout)
	}
	if out.Phone != "13800138000" {
		t.Fatalf("Phone = %q, want 13800138000", out.Phone)
	}
}

func TestParseTextFallsBackToFirstBareLine(t *testing.T) {
	out := parseText("幸福里\n租金：4500")
	if out.CommunityName != "幸福里" {
		t.Fatalf("CommunityName = %q, want 幸福里", out.CommunityName)
	}
	if out.RentYuan != 4500 {
		t.Fatalf("RentYuan = %v, want 4500", out.RentYuan)
	}
}

func TestParsedPhoneHashesLikeStoredLandlordPhone(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	pii, err := secure.NewPII(key, "test-hash-key")
	if err != nil {
		t.Fatalf("NewPII: %v", err)
	}
	extracted := parseText("小区：幸福里\n电话 13800138000")
	if extracted.Phone != "13800138000" {
		t.Fatalf("Phone = %q, want 13800138000", extracted.Phone)
	}
	if got, want := pii.HashPhone(extracted.Phone), pii.HashPhone("138 0013 8000"); !bytes.Equal(got, want) {
		t.Fatalf("phone hash mismatch: %x != %x", got, want)
	}
}

func TestClassifyMediaAcceptsDocsAndRejectsLegacyFormats(t *testing.T) {
	cases := []struct {
		filename string
		mime     string
		kind     string
		ext      string
		max      int
	}{
		{"photo.JPG", "", "image", "jpg", maxImageBytes},
		{"clip.mov", "", "video", "mov", maxVideoBytes},
		{"notes.docx", "", "doc", "docx", maxDocBytes},
		{"readme.md", "", "doc", "md", maxDocBytes},
		{"upload", "text/plain", "doc", "txt", maxDocBytes},
		{"scan.pdf", "application/pdf", "", "", 0},
		{"legacy.doc", "application/msword", "", "", 0},
	}
	for _, tc := range cases {
		kind, ext, max := classifyMedia(tc.filename, tc.mime)
		if kind != tc.kind || ext != tc.ext || max != tc.max {
			t.Fatalf("classifyMedia(%q, %q) = (%q, %q, %d), want (%q, %q, %d)",
				tc.filename, tc.mime, kind, ext, max, tc.kind, tc.ext, tc.max)
		}
	}
}

func TestExtractDocTextPlainAndDocx(t *testing.T) {
	md := []byte("\xEF\xBB\xBF# 幸福里\n三室两厅")
	text, err := extractDocText("md", md)
	if err != nil {
		t.Fatalf("extractDocText(md): %v", err)
	}
	if text != "# 幸福里\n三室两厅" {
		t.Fatalf("md text = %q", text)
	}
	if _, err := extractDocText("txt", []byte("   ")); err == nil {
		t.Fatal("expected empty document error")
	}
	if _, err := extractDocText("txt", []byte{0xff, 0xfe}); err == nil {
		t.Fatal("expected invalid UTF-8 error")
	}

	docx := buildDocx(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>幸福里</w:t></w:r></w:p><w:p><w:r><w:t>89 平</w:t></w:r></w:p></w:body></w:document>`)
	text, err = extractDocText("docx", docx)
	if err != nil {
		t.Fatalf("extractDocText(docx): %v", err)
	}
	if text != "幸福里\n89 平" {
		t.Fatalf("docx text = %q", text)
	}
}

func buildDocx(t *testing.T, documentXML string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := w.Write([]byte(documentXML)); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}
