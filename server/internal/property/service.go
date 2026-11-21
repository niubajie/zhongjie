package property

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"housing/internal/building"
	"housing/internal/community"
	"housing/internal/storage"
	"housing/internal/user"
)

var phonePattern = regexp.MustCompile(`1[3-9]\d{9}`)

var (
	ErrForbidden         = errors.New("only landlords can publish")
	ErrBuildingNotFound  = errors.New("building not found")
	ErrCommunityNotFound = errors.New("community not found")
	ErrCommunityRequired = errors.New("residential community is required")
	ErrInvalidInput      = errors.New("invalid property input")
)

type Service struct {
	repo        *Repository
	buildings   *building.Service
	communities *community.Service
	storage     *storage.Service
	users       *user.Repository
}

func NewService(repo *Repository, buildings *building.Service, communities *community.Service, media *storage.Service, users *user.Repository) *Service {
	return &Service{repo: repo, buildings: buildings, communities: communities, storage: media, users: users}
}

func (s *Service) Map(ctx context.Context, q MapQuery) (MapResult, error) {
	q.CityCode = strings.TrimSpace(q.CityCode)
	if q.CityCode == "" {
		return MapResult{}, errors.New("cityCode is required")
	}
	if q.Zoom < 3 || q.Zoom > 16 {
		return MapResult{}, errors.New("zoom must be between 3 and 16")
	}
	if q.SWLat < -90 || q.NELat > 90 || q.SWLat >= q.NELat {
		return MapResult{}, errors.New("invalid latitude bounds")
	}
	if q.SWLng < -180 || q.NELng > 180 || q.SWLng >= q.NELng {
		return MapResult{}, errors.New("invalid longitude bounds")
	}
	return s.repo.Map(ctx, q)
}

// ListMine returns the landlord's own listings with freshness state.
func (s *Service) ListMine(ctx context.Context, landlordID int64, limit int) ([]MineItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.ListByLandlord(ctx, landlordID, limit)
}

func (s *Service) Detail(ctx context.Context, uuid string) (Detail, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return Detail{}, fmt.Errorf("uuid is required")
	}
	detail, err := s.repo.Detail(ctx, uuid)
	if err != nil {
		return Detail{}, err
	}
	if s.storage != nil {
		for i := range detail.Images {
			detail.Images[i].URL = s.storage.URL(detail.Images[i].ObjectKey)
		}
	}
	return detail, nil
}

const defaultCityCode = "440300"

func (s *Service) Create(ctx context.Context, userID int64, in CreateInput) (Created, error) {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return Created{}, err
	}
	if u.Role != "landlord" || u.Status != "active" {
		return Created{}, ErrForbidden
	}
	if in.AreaSqm < 5 || in.AreaSqm > 1000 {
		return Created{}, fmt.Errorf("%w: areaSqm must be 5–1000", ErrInvalidInput)
	}
	if in.RentCents < 10_000 || in.RentCents > 100_000_000 {
		return Created{}, fmt.Errorf("%w: rentCents must be 100–1,000,000 yuan", ErrInvalidInput)
	}

	place, err := s.resolvePlace(ctx, in)
	if err != nil {
		return Created{}, err
	}

	in.Layout = clip(in.Layout, 40)
	in.Floor = clip(in.Floor, 40)
	in.Orientation = clip(in.Orientation, 40)

	placeName := place.CommunityName
	if placeName == "" {
		placeName = place.BuildingName
	}
	in.Description = strings.ReplaceAll(in.Description, placeName, "该小区")
	in.Description = phonePattern.ReplaceAllString(in.Description, "[联系方式已隐藏]")
	in.Description = clip(in.Description, 2000)

	title := strings.TrimSpace(in.Title)
	if placeName != "" {
		title = strings.ReplaceAll(title, placeName, "房源")
	}
	title = phonePattern.ReplaceAllString(title, "[联系方式已隐藏]")
	title = strings.TrimSpace(title)
	if title == "" {
		title = fmt.Sprintf("%s · %.0f㎡ · %d元/月", placeTypeLabel(place.BuildingType), in.AreaSqm, in.RentCents/100)
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

	// v1.4 hard gate: only a matched residential community may auto-publish.
	status := "pending"
	pendingReason := "missing_community_name"
	if place.CommunityID != nil {
		status = "active"
		pendingReason = ""
	}
	return s.repo.Create(ctx, userID, place, in, status, pendingReason)
}

func (s *Service) resolvePlace(ctx context.Context, in CreateInput) (PublishPlace, error) {
	if uuid := strings.TrimSpace(in.CommunityUUID); uuid != "" {
		c, err := s.communities.Get(ctx, uuid)
		if errors.Is(err, community.ErrNotFound) {
			return PublishPlace{}, ErrCommunityNotFound
		}
		if err != nil {
			return PublishPlace{}, err
		}
		return placeFromCommunity(c), nil
	}
	if name := strings.TrimSpace(in.CommunityName); name != "" {
		c, err := s.communities.FindByName(ctx, defaultCityCode, name)
		if errors.Is(err, community.ErrNotFound) {
			return PublishPlace{}, ErrCommunityNotFound
		}
		if err != nil {
			return PublishPlace{}, err
		}
		return placeFromCommunity(c), nil
	}
	if uuid := strings.TrimSpace(in.BuildingUUID); uuid != "" {
		b, err := s.buildings.Get(ctx, uuid)
		if errors.Is(err, building.ErrNotFound) {
			return PublishPlace{}, ErrBuildingNotFound
		}
		if err != nil {
			return PublishPlace{}, err
		}
		return PublishPlace{
			CityCode: b.CityCode, DistrictCode: b.DistrictCode, DistrictName: b.DistrictName,
			StreetCode: b.StreetCode, StreetName: b.StreetName, Lat: b.Lat, Lng: b.Lng,
			BuildingID: &b.ID, BuildingName: b.Name, BuildingType: b.BuildingType,
		}, nil
	}
	return PublishPlace{}, fmt.Errorf("%w: 请选择小区", ErrCommunityRequired)
}

func placeFromCommunity(c community.Community) PublishPlace {
	return PublishPlace{
		CityCode: c.CityCode, DistrictCode: c.DistrictCode, DistrictName: c.DistrictName,
		StreetCode: c.StreetCode, StreetName: c.StreetName, Lat: c.Lat, Lng: c.Lng,
		BuildingName: c.Name, BuildingType: c.Type,
		CommunityID: &c.ID, CommunityName: c.Name,
	}
}

func placeTypeLabel(t string) string {
	switch t {
	case "industrial_park":
		return "工业园区"
	case "office_building":
		return "写字楼"
	case "apartment":
		return "公寓"
	default:
		return "住宅"
	}
}

func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}
