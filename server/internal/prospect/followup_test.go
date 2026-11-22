package prospect

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPlanFollowUpReminder(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		lastActive time.Time
		wantLevel  string
		wantStale  bool
	}{
		{"刚跟进", now.Add(-2 * time.Hour), "", false},
		{"差一秒到 3 天", now.Add(-FollowUpDueAfter + time.Second), "", false},
		{"刚好 3 天", now.Add(-FollowUpDueAfter), ReminderLevelDue3d, false},
		{"6 天", now.Add(-6 * 24 * time.Hour), ReminderLevelDue3d, false},
		{"刚好 7 天", now.Add(-FollowUpOverdueAfter), ReminderLevelOverdue7d, true},
		{"10 天", now.Add(-10 * 24 * time.Hour), ReminderLevelOverdue7d, true},
		{"未来时间", now.Add(time.Hour), "", false},
	}
	for _, c := range cases {
		level, stale := planFollowUpReminder(c.lastActive, now)
		if level != c.wantLevel || stale != c.wantStale {
			t.Errorf("%s: planFollowUpReminder = (%q,%v), want (%q,%v)",
				c.name, level, stale, c.wantLevel, c.wantStale)
		}
	}
	if level, stale := planFollowUpReminder(time.Time{}, now); level != "" || stale {
		t.Errorf("零值时间应不产生提醒, got (%q,%v)", level, stale)
	}
}

func TestValidateFollowUpInput(t *testing.T) {
	cases := []struct {
		name          string
		in            AddFollowUpInput
		wantDirection string
		wantStatus    string
	}{
		{"默认出站", AddFollowUpInput{Channel: ChannelPhone, Content: "回访"}, DirectionOutbound, followUpStatusSent},
		{"显式出站", AddFollowUpInput{Channel: ChannelWeChat, Direction: DirectionOutbound, Content: "回访"}, DirectionOutbound, followUpStatusSent},
		{"入站", AddFollowUpInput{Channel: ChannelSMS, Direction: DirectionInbound, Content: "客户回复"}, DirectionInbound, followUpStatusReceived},
	}
	for _, c := range cases {
		channel, direction, status, err := validateFollowUpInput(c.in)
		if err != nil {
			t.Errorf("%s: 不应报错, got %v", c.name, err)
			continue
		}
		if channel != c.in.Channel || direction != c.wantDirection || status != c.wantStatus {
			t.Errorf("%s: got (%q,%q,%q), want (%q,%q,%q)",
				c.name, channel, direction, status, c.in.Channel, c.wantDirection, c.wantStatus)
		}
	}

	invalid := []AddFollowUpInput{
		{Channel: ChannelUnknown, Content: "x"},
		{Channel: "email", Content: "x"},
		{Channel: ChannelPhone, Content: ""},
		{Channel: ChannelPhone, Content: "   "},
		{Channel: ChannelPhone, Direction: "sideways", Content: "x"},
	}
	for _, in := range invalid {
		if _, _, _, err := validateFollowUpInput(in); !errors.Is(err, ErrInvalidFollowUp) {
			t.Errorf("validateFollowUpInput(%+v) 应返回 ErrInvalidFollowUp, got %v", in, err)
		}
	}
}

func TestAddFollowUpValidatesBeforeRepo(t *testing.T) {
	s := &Service{}
	// 参数非法时必须在访问 repository 前返回，允许在无数据库环境下验证。
	if _, err := s.AddFollowUp(context.Background(), 0, "uuid", AddFollowUpInput{Channel: "bad", Content: "x"}); !errors.Is(err, ErrInvalidFollowUp) {
		t.Errorf("非法渠道应返回 ErrInvalidFollowUp, got %v", err)
	}
}

func TestToLeadVOStale(t *testing.T) {
	vo := toLeadVO(ProspectLead{Status: StatusFollowingUp, StaleAt: "2026-11-22T10:00:00Z"})
	if !vo.Stale || vo.StaleAt != "2026-11-22T10:00:00Z" {
		t.Errorf("超期字段映射错误: %+v", vo)
	}
	normal := toLeadVO(ProspectLead{Status: StatusFollowingUp})
	if normal.Stale || normal.StaleAt != "" {
		t.Errorf("未超期线索不应带 stale: %+v", normal)
	}
}
