package intake

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const maxDocBytes = 10 << 20

// extractDocText reads plain text formats and docx without any third-party
// dependency. Scanned PDFs and the legacy .doc format are rejected upstream
// with an explicit hint instead of guessing.
func extractDocText(ext string, data []byte) (string, error) {
	if ext == "docx" {
		return extractDocxText(data)
	}
	return extractPlainText(data)
}

// extractDocxText unzips word/document.xml and joins the text runs, turning
// paragraphs into newlines.
func extractDocxText(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", errors.New("不是有效的 docx 压缩包")
	}
	var entry *zip.File
	for _, f := range zr.File {
		// Some zip writers (e.g. Windows Compress-Archive) use backslashes.
		if strings.ReplaceAll(f.Name, "\\", "/") == "word/document.xml" {
			entry = f
			break
		}
	}
	if entry == nil {
		return "", errors.New("docx 缺少 word/document.xml")
	}
	rc, err := entry.Open()
	if err != nil {
		return "", fmt.Errorf("打开文档正文失败: %w", err)
	}
	defer rc.Close()
	return decodeWordXML(io.LimitReader(rc, 20<<20))
}

func decodeWordXML(r io.Reader) (string, error) {
	dec := xml.NewDecoder(r)
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("解析文档正文失败: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				var text string
				if err := dec.DecodeElement(&text, &t); err != nil {
					return "", fmt.Errorf("解析文档文本失败: %w", err)
				}
				b.WriteString(text)
			case "tab":
				b.WriteString(" ")
			case "br", "cr":
				b.WriteString("\n")
			}
		case xml.EndElement:
			if t.Name.Local == "p" {
				b.WriteString("\n")
			}
		}
	}
	return strings.TrimSpace(b.String()), nil
}

func extractPlainText(data []byte) (string, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if len(bytes.TrimSpace(data)) == 0 {
		return "", errors.New("文档为空")
	}
	if !utf8.Valid(data) {
		return "", errors.New("文本不是 UTF-8 编码，请另存为 UTF-8 或 docx")
	}
	return strings.TrimSpace(string(data)), nil
}
