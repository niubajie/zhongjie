package building

import (
	"context"
	"errors"
	"strings"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Suggest(ctx context.Context, cityCode, keyword string) ([]SuggestItem, error) {
	cityCode = strings.TrimSpace(cityCode)
	keyword = strings.TrimSpace(keyword)
	if cityCode == "" {
		return nil, errors.New("cityCode is required")
	}
	if len([]rune(keyword)) > 50 {
		keyword = string([]rune(keyword)[:50])
	}
	items, err := s.repo.Suggest(ctx, cityCode, keyword, 10)
	if err != nil {
		return nil, err
	}
	out := make([]SuggestItem, 0, len(items))
	for _, b := range items {
		out = append(out, b.Suggest())
	}
	return out, nil
}

func (s *Service) FindByName(ctx context.Context, cityCode, name string) (Building, error) {
	return s.repo.FindByName(ctx, strings.TrimSpace(cityCode), strings.TrimSpace(name))
}

func (s *Service) Get(ctx context.Context, uuid string) (Building, error) {
	if strings.TrimSpace(uuid) == "" {
		return Building{}, ErrNotFound
	}
	return s.repo.GetByUUID(ctx, uuid)
}
