package prospect

import "testing"

func TestValidStatus(t *testing.T) {
	valid := []string{
		StatusNew, StatusReviewed, StatusContacted, StatusFollowingUp,
		StatusConverted, StatusPublished, StatusCold, StatusLost,
		StatusOptOut, StatusRejected, StatusIgnored,
	}
	for _, s := range valid {
		if !ValidStatus(s) {
			t.Errorf("ValidStatus(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "new ", "review", "deleted", "active"}
	for _, s := range invalid {
		if ValidStatus(s) {
			t.Errorf("ValidStatus(%q) = true, want false", s)
		}
	}
}

func TestCanTransition(t *testing.T) {
	cases := []struct {
		name string
		from string
		to   string
		want bool
	}{
		{"new to reviewed", StatusNew, StatusReviewed, true},
		{"new to contacted (skip review)", StatusNew, StatusContacted, false},
		{"new to rejected", StatusNew, StatusRejected, true},
		{"reviewed to contacted", StatusReviewed, StatusContacted, true},
		{"contacted to following_up", StatusContacted, StatusFollowingUp, true},
		{"contacted to converted", StatusContacted, StatusConverted, true},
		{"following_up to cold", StatusFollowingUp, StatusCold, true},
		{"same state no-op", StatusNew, StatusNew, false},
		{"terminal cannot leave", StatusConverted, StatusNew, false},
		{"rejected cannot reopen", StatusRejected, StatusReviewed, false},
		{"unknown from", "ghost", StatusNew, false},
		{"unknown to", StatusNew, "ghost", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanTransition(tc.from, tc.to); got != tc.want {
				t.Fatalf("CanTransition(%q, %q) = %v, want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

func TestTerminalStatusesClosed(t *testing.T) {
	// 每个终态都不应出现在 allowedTransitions 的键中（不可转出）。
	for s := range terminalStatuses {
		if _, ok := allowedTransitions[s]; ok {
			t.Errorf("terminal status %q must not allow outgoing transitions", s)
		}
	}
}

func TestParseBoolSetting(t *testing.T) {
	cases := map[string]bool{
		"true":  true,
		"false": false,
		"":      false,
		"1":     false,
		"TRUE":  false,
	}
	for in, want := range cases {
		if got := parseBoolSetting(in); got != want {
			t.Errorf("parseBoolSetting(%q) = %v, want %v", in, got, want)
		}
	}
}
