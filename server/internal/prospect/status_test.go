package prospect

import (
	"context"
	"errors"
	"testing"
)

func TestValidAdminTargetStatus(t *testing.T) {
	allowed := []string{
		StatusFollowingUp, StatusConverted, StatusPublished,
		StatusCold, StatusLost, StatusRejected, StatusIgnored,
	}
	for _, s := range allowed {
		if !ValidAdminTargetStatus(s) {
			t.Errorf("ValidAdminTargetStatus(%q) = false, want true", s)
		}
	}
	// 专用接口状态与未知值不允许走通用流转。
	denied := []string{StatusNew, StatusReviewed, StatusContacted, StatusOptOut, "", "ghost"}
	for _, s := range denied {
		if ValidAdminTargetStatus(s) {
			t.Errorf("ValidAdminTargetStatus(%q) = true, want false", s)
		}
	}
}

func TestTransitionLeadRejectsSpecialStates(t *testing.T) {
	s := &Service{}
	ctx := context.Background()
	for _, target := range []string{StatusReviewed, StatusContacted, StatusOptOut} {
		if _, err := s.TransitionLead(ctx, 0, "uuid", target); !errors.Is(err, ErrInvalidStatus) {
			t.Errorf("TransitionLead(%q) 应返回 ErrInvalidStatus, got %v", target, err)
		}
	}
	if _, err := s.TransitionLead(ctx, 0, "uuid", "ghost"); !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("非法目标状态应返回 ErrInvalidStatus, got %v", err)
	}
}

func TestListOptOutsValidatesLeadType(t *testing.T) {
	s := &Service{}
	if _, err := s.ListOptOuts(context.Background(), "agent", 10); !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("非法 leadType 应返回 ErrInvalidStatus, got %v", err)
	}
}
