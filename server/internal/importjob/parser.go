package importjob

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

func Parse(in FileInput) ([]Row, error) {
	switch strings.ToLower(strings.TrimPrefix(in.Ext, ".")) {
	case "xlsx":
		return parseXLSX(in.Data)
	case "docx":
		return parseDOCX(in.Data)
	default:
		return nil, ErrUnsupportedType
	}
}

func parseXLSX(data []byte) ([]Row, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, ErrNoRows
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, ErrNoRows
	}

	header := make(map[string]int)
	for i, cell := range rows[0] {
		key := normalizeHeader(cell)
		if key != "" {
			header[key] = i
		}
	}
	col := func(row []string, names ...string) string {
		for _, name := range names {
			if idx, ok := header[name]; ok && idx < len(row) {
				return strings.TrimSpace(row[idx])
			}
		}
		return ""
	}

	out := make([]Row, 0, len(rows)-1)
	for _, row := range rows[1:] {
		building := col(row, "楼宇", "楼宇名称", "大厦", "园区", "项目")
		if building == "" {
			continue
		}
		area := parseNumber(col(row, "面积", "建筑面积"))
		rent := parseNumber(col(row, "租金", "月租", "价格"))
		if area <= 0 || rent <= 0 {
			continue
		}
		out = append(out, Row{
			BuildingName: building,
			AreaSqm:      area,
			RentYuan:     rent,
			Layout:       col(row, "户型", "房型"),
			Title:        col(row, "标题", "房源标题"),
		})
	}
	if len(out) == 0 {
		return nil, ErrNoRows
	}
	return out, nil
}

func parseDOCX(data []byte) ([]Row, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open docx: %w", err)
	}
	var doc *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			doc = f
			break
		}
	}
	if doc == nil {
		return nil, errors.New("docx document.xml not found")
	}
	rc, err := doc.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	var b strings.Builder
	inText := false
	decoder := xml.NewDecoder(io.LimitReader(rc, 5<<20))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" {
				inText = true
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inText = false
			}
			if t.Name.Local == "p" {
				b.WriteString("\n")
			}
		case xml.CharData:
			if inText {
				b.Write(t)
			}
		}
	}

	lines := strings.Split(b.String(), "\n")
	row := Row{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case hasPrefix(line, "楼宇", "大厦", "园区", "项目"):
			row.BuildingName = valueAfterColon(line)
		case hasPrefix(line, "面积"):
			row.AreaSqm = parseNumber(valueAfterColon(line))
		case hasPrefix(line, "租金", "月租", "价格"):
			row.RentYuan = parseNumber(valueAfterColon(line))
		case hasPrefix(line, "户型", "房型"):
			row.Layout = valueAfterColon(line)
		case hasPrefix(line, "标题"):
			row.Title = valueAfterColon(line)
		}
	}
	if row.BuildingName == "" || row.AreaSqm <= 0 || row.RentYuan <= 0 {
		return nil, ErrNoRows
	}
	return []Row{row}, nil
}

func normalizeHeader(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "（", "(")
	s = strings.ReplaceAll(s, "）", ")")
	for _, suffix := range []string{"(㎡)", "(m2)", "(元/月)", "(元)", "(平方米)"} {
		s = strings.ReplaceAll(s, suffix, "")
	}
	return s
}

func parseNumber(s string) float64 {
	var b strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' {
			b.WriteRune(r)
		}
	}
	v, _ := strconv.ParseFloat(b.String(), 64)
	return v
}

func hasPrefix(line string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(strings.ReplaceAll(line, " ", ""), p) {
			return true
		}
	}
	return false
}

func valueAfterColon(line string) string {
	line = strings.ReplaceAll(line, "：", ":")
	if _, value, ok := strings.Cut(line, ":"); ok {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(line)
}
