package property

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"housing/internal/building"
	"housing/internal/user"
)

var phonePattern = regexp.MustCompile(`1[3-9]\d{9}`)

var (
	ErrForbidden        = errors.New("only landlords can publish")
	ErrBuildingNotFound = errors.New("building not found")
	ErrInvalidInput     = errors.New("invalid property input")
)

type Service struct {
	repo      *Repository
	buildings *building.Service
	users     *user.Repository
}

func NewService(repo *Repository, buildings *building.Service, users *user.Repository) *Service {
	return &Service{repo: repo, buildings: buildings, users: users}
}

func (s *Service) Map(ctx context.Context, q MapQuery) (MapResult, error) {
	q.CityCode = strings.TrimSpace(q.CityCode)
	if q.CityCode == "" {
		return MapResult{}, errors.New("cityCode is required")
	}
	if q.Zoom < 3 || q.Zoom > 20 {
		return MapResult{}, errors.New("zoom must be between 3 and 20")
	}
	if q.SWLat < -90 || q.NELat > 90 || q.SWLat >= q.NELat {
		return MapResult{}, errors.New("invalid latitude bounds")
	}
	if q.SWLng < -180 || q.NELng > 180 || q.SWLng >= q.NELng {
		return MapResult{}, errors.New("invalid longitude bounds")
	}
	return s.repo.Map(ctx, q)
}

func (s *Service) Detail(ctx context.Context, uuid string) (Detail, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return Detail{}, fmt.Errorf("uuid is required")
	}
	return s.repo.Detail(ctx, uuid)
}

func (s *Service) Create(ctx context.Context, userID int64, in CreateInput) (Created, error) {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return Created{}, err
	}
	if u.Role != "landlord" || u.Status != "active" {
		return Created{}, ErrForbidden
	}
	if utf8.RuneCountInString(strings.TrimSpace(in.BuildingUUID)) == 0 {
		return Created{}, fmt.Errorf("%w: buildingUuid is required", ErrInvalidInput)
	}
	if in.AreaSqm < 5 || in.AreaSqm > 1000 {
		return Created{}, fmt.Errorf("%w: areaSqm must be 5–1000", ErrInvalidInput)
	}
	if in.RentCents < 10_000 || in.RentCents > 100_000_000 {
		return Created{}, fmt.Errorf("%w: rentCents must be 100–1,000,000 yuan", ErrInvalidInput)
	}

	b, err := s.buildings.Get(ctx, strings.TrimSpace(in.BuildingUUID))
	if errors.Is(err, building.ErrNotFound) {
		return Created{}, ErrBuildingNotFound
	}
	if err != nil {
		return Created{}, err
	}

	in.Layout = clip(in.Layout, 40)
	in.Floor = clip(in.Floor, 40)
	in.Orientation = clip(in.Orientation, 40)

	in.Description = strings.ReplaceAll(in.Description, b.Name, "该楼宇")
	in.Description = phonePattern.ReplaceAllString(in.Description, "[联系方式已隐藏]")
	in.Description = clip(in.Description, 2000)

	title := strings.TrimSpace(in.Title)
	title = strings.ReplaceAll(title, b.Name, "房源")
	title = phonePattern.ReplaceAllString(title, "[联系方式已隐藏]")
	title = strings.TrimSpace(title)
	if title == "" {
		title = fmt.Sprintf("%s · %.0f㎡ · %d元/月", buildingTypeLabel(b.BuildingType), in.AreaSqm, in.RentCents/100)
	}
	in.Title = clip(title, 80)

	if len(in.ImageKeys) > 9 {
		return Created{}, fmt.Errorf("%w: at most 9 images", ErrInvalidInput)
	}
	keys := make([]string, 0, len(in.ImageKeys))
	for _, key := range in.ImageKeys {
		key = clip(key, 500)
		if key != "" {
			keys = append(keys, key)
		}
	}
	in.ImageKeys = keys

	status := "active"
	pendingReason := ""
	if b.ReviewStatus != "approved" {
		status = "pending"
		pendingReason = "building_review_pending"
	}
	return s.repo.Create(ctx, userID, b, in, status, pendingReason)
}

func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

func buildingTypeLabel(t string) string {
	if t == "industrial_park" {
		return "工业园区"
	}
	return "写字楼"
}
