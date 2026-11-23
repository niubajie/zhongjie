package prospect

import (
	"errors"
	"testing"
)

func TestSourceConversionRate(t *testing.T) {
	cases := []struct {
		converted int
		total     int
		want      float64
	}{
		{0, 0, 0},
		{0, 5, 0},
		{1, 3, 0.3333},
		{2, 2, 1},
		{3, 4, 0.75},
	}
	for _, tc := range cases {
		if got := sourceConversionRate(tc.converted, tc.total); got != tc.want {
			t.Fatalf("sourceConversionRate(%d, %d) = %v, want %v", tc.converted, tc.total, got, tc.want)
		}
	}
}

func TestParseStatsRange(t *testing.T) {
	from, to, err := parseStatsRange(SourceStatsQuery{})
	if err != nil || from != nil || to != nil {
		t.Fatalf("空区间应返回 nil, nil, nil，实际 %v, %v, %v", from, to, err)
	}

	from, to, err = parseStatsRange(SourceStatsQuery{
		From: "2026-11-01T00:00:00Z",
		To:   "2026-11-30T23:59:59Z",
	})
	if err != nil || from == nil || to == nil {
		t.Fatalf("合法区间解析失败：%v", err)
	}
	if got := from.Format("2006-01-02"); got != "2026-11-01" {
		t.Fatalf("from 解析错误：%s", got)
	}
	if got := to.Format("2006-01-02"); got != "2026-11-30" {
		t.Fatalf("to 解析错误：%s", got)
	}

	if _, _, err := parseStatsRange(SourceStatsQuery{From: "2026-11-30T00:00:00Z", To: "2026-11-01T00:00:00Z"}); !errors.Is(err, ErrInvalidStatsQuery) {
		t.Fatalf("结束早于开始应返回 ErrInvalidStatsQuery，实际 %v", err)
	}

	if _, _, err := parseStatsRange(SourceStatsQuery{From: "not-a-time"}); !errors.Is(err, ErrInvalidStatsQuery) {
		t.Fatalf("非法时间应返回 ErrInvalidStatsQuery，实际 %v", err)
	}
}
