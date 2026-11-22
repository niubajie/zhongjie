package prospect

import "testing"

func TestConversionRate(t *testing.T) {
	cases := []struct {
		converted int64
		contacted int64
		want      float64
	}{
		{0, 0, 0},
		{0, 5, 0},
		{5, 5, 1},
		{1, 3, 0.3333},
		{2, 3, 0.6667},
		{3, 2, 1.5},
	}
	for _, c := range cases {
		if got := conversionRate(c.converted, c.contacted); got != c.want {
			t.Errorf("conversionRate(%d,%d) = %v, want %v", c.converted, c.contacted, got, c.want)
		}
	}
}
