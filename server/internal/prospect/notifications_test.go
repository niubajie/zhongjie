package prospect

import "testing"

// 批次11：提醒级别 → 站内通知事件类型与文案映射。
func TestReminderNotification(t *testing.T) {
	cases := []struct {
		level     string
		eventType string
		title     string
	}{
		{ReminderLevelDue3d, AdminEventFollowUpDue, "获客跟进待处理"},
		{ReminderLevelOverdue7d, AdminEventFollowUpOverdue, "获客跟进已超期"},
		{"", AdminEventFollowUpDue, "获客跟进待处理"},
	}
	for _, tc := range cases {
		eventType, title, body := reminderNotification(tc.level)
		if eventType != tc.eventType {
			t.Fatalf("level %q: eventType = %q, want %q", tc.level, eventType, tc.eventType)
		}
		if title != tc.title {
			t.Fatalf("level %q: title = %q, want %q", tc.level, title, tc.title)
		}
		if body == "" {
			t.Fatalf("level %q: body 不能为空", tc.level)
		}
	}
}
