package prospect

import (
	"context"
	"errors"
	"testing"
)

func TestBatchActionValidation(t *testing.T) {
	svc := &Service{}
	cases := []struct {
		name string
		in   BatchActionInput
	}{
		{"未选择线索", BatchActionInput{Action: "approve"}},
		{"超过上限", BatchActionInput{Action: "approve", UUIDs: make([]string, MaxBatchLeads+1)}},
		{"未知操作", BatchActionInput{Action: "delete", UUIDs: []string{"a"}}},
		{"流转缺目标状态", BatchActionInput{Action: "transition", UUIDs: []string{"a"}}},
		{"全部为空白", BatchActionInput{Action: "approve", UUIDs: []string{" ", ""}}},
	}
	for _, tc := range cases {
		if _, err := svc.BatchAction(context.Background(), 1, tc.in); !errors.Is(err, ErrInvalidBatchInput) {
			t.Fatalf("%s: 期望 ErrInvalidBatchInput，实际 %v", tc.name, err)
		}
	}
}
