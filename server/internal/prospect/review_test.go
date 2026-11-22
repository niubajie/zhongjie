package prospect

import (
	"errors"
	"testing"
)

func TestReviewTargetStatus(t *testing.T) {
	cases := []struct {
		decision string
		want     string
	}{
		{ReviewDecisionApprove, StatusReviewed},
		{ReviewDecisionReject, StatusRejected},
	}
	for _, c := range cases {
		got, err := reviewTargetStatus(c.decision)
		if err != nil {
			t.Fatalf("reviewTargetStatus(%q) error: %v", c.decision, err)
		}
		if got != c.want {
			t.Errorf("reviewTargetStatus(%q) = %q, want %q", c.decision, got, c.want)
		}
	}
}

func TestReviewTargetStatusRejectsInvalid(t *testing.T) {
	for _, decision := range []string{"", "approved", "pass", "REJECT"} {
		if _, err := reviewTargetStatus(decision); !errors.Is(err, ErrInvalidReview) {
			t.Errorf("reviewTargetStatus(%q) 应返回 ErrInvalidReview, got %v", decision, err)
		}
	}
}

func TestValidateContactInput(t *testing.T) {
	valid := []ContactLeadInput{
		{Channel: ChannelInApp, Content: "您好，看到您在找房"},
		{Channel: ChannelSMS, Content: "content"},
		{Channel: ChannelWeChat, Content: "content"},
		{Channel: ChannelPhone, Content: "content"},
	}
	for _, in := range valid {
		if err := validateContactInput(in); err != nil {
			t.Errorf("validateContactInput(%+v) 应通过, got %v", in, err)
		}
	}

	invalid := []ContactLeadInput{
		{Channel: ChannelUnknown, Content: "content"},
		{Channel: "email", Content: "content"},
		{Channel: ChannelPhone, Content: ""},
		{Channel: ChannelPhone, Content: "   "},
	}
	for _, in := range invalid {
		if err := validateContactInput(in); !errors.Is(err, ErrInvalidContact) {
			t.Errorf("validateContactInput(%+v) 应返回 ErrInvalidContact, got %v", in, err)
		}
	}
}

func TestToFollowUpVO(t *testing.T) {
	f := FollowUp{
		ID: 12, Channel: ChannelPhone, Direction: DirectionOutbound, Status: "sent",
		SentAt: "2026-11-22T10:00:00Z", CreatedAt: "2026-11-22T10:00:00Z",
	}
	vo := toFollowUpVO(f, "您好")
	if vo.ID != 12 || vo.Channel != ChannelPhone || vo.Direction != DirectionOutbound ||
		vo.Status != "sent" || vo.Content != "您好" || vo.SentAt != f.SentAt || vo.CreatedAt != f.CreatedAt {
		t.Errorf("toFollowUpVO 字段映射错误: %+v", vo)
	}
}
