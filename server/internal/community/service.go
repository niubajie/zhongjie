package community

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
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
	if utf8.RuneCountInString(keyword) > 50 {
		keyword = string([]rune(keyword)[:50])
	}
	items, err := s.repo.Suggest(ctx, cityCode, keyword, 10)
	if err != nil {
		return nil, err
	}
	out := make([]SuggestItem, 0, len(items))
	for _, c := range items {
		out = append(out, c.Suggest())
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, uuid string) (Community, error) {
	if strings.TrimSpace(uuid) == "" {
		return Community{}, ErrNotFound
	}
	return s.repo.GetByUUID(ctx, strings.TrimSpace(uuid))
}

func (s *Service) FindByName(ctx context.Context, cityCode, name string) (Community, error) {
	return s.repo.FindByName(ctx, strings.TrimSpace(cityCode), strings.TrimSpace(name))
}

func (s *Service) Create(ctx context.Context, adminID int64, in CreateInput) (Community, error) {
	in.CityCode = strings.TrimSpace(in.CityCode)
	in.Name = strings.TrimSpace(in.Name)
	in.DistrictCode = strings.TrimSpace(in.DistrictCode)
	in.DistrictName = strings.TrimSpace(in.DistrictName)
	in.StreetCode = strings.TrimSpace(in.StreetCode)
	in.StreetName = strings.TrimSpace(in.StreetName)
	if in.CityCode == "" || in.Name == "" || in.DistrictCode == "" || in.StreetCode == "" {
		return Community{}, errors.New("cityCode、name、districtCode、streetCode 必填")
	}
	if utf8.RuneCountInString(in.Name) > 80 {
		return Community{}, errors.New("小区名称过长")
	}
	if in.Type == "" {
		in.Type = "residential"
	}
	if in.Type != "residential" && in.Type != "apartment" {
		return Community{}, errors.New("communityType 只能是 residential 或 apartment")
	}
	if in.Lat < -90 || in.Lat > 90 || in.Lng < -180 || in.Lng > 180 {
		return Community{}, errors.New("经纬度不合法")
	}
	aliases := make([]string, 0, len(in.Aliases))
	for _, alias := range in.Aliases {
		if alias = strings.TrimSpace(alias); alias != "" {
			aliases = append(aliases, alias)
		}
	}
	in.Aliases = aliases
	return s.repo.Create(ctx, adminID, in, "manual")
}
