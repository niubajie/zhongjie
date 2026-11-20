package freshness

import (
	"context"
	"strings"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Respond(ctx context.Context, userID int64, uuid, status string) (string, error) {
	status = strings.TrimSpace(status)
	switch status {
	case "active", "rented", "offline":
	default:
		return "", ErrInvalidStatus
	}
	return s.repo.Respond(ctx, userID, strings.TrimSpace(uuid), status)
}

func (s *Service) Run(ctx context.Context) (DueResult, error) {
	n, err := s.repo.CreateDueChecks(ctx)
	if err != nil {
		return DueResult{}, err
	}
	return DueResult{Created: n}, nil
}
