package intake

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"housing/internal/building"
	"housing/internal/property"
	"housing/internal/secure"
	"housing/internal/user"
)

var (
	buildingRe = regexp.MustCompile(`(?:楼宇|大厦|园区|项目|地址)\s*[:：]\s*(.+)`)
	areaRe     = regexp.MustCompile(`面积\s*[:：]?\s*(\d+(?:\.\d+)?)`)
	rentRe     = regexp.MustCompile(`(?:租金|月租|价格)\s*[:：]?\s*(\d+(?:\.\d+)?)`)
	layoutRe   = regexp.MustCompile(`(?:户型|房型)\s*[:：]?\s*([0-9+]+室?[0-9]*厅?)`)
)

type Extracted struct {
	BuildingName string  `json:"buildingName"`
	AreaSqm      float64 `json:"areaSqm"`
	RentYuan     float64 `json:"rentYuan"`
	Layout       string  `json:"layout"`
	Title        string  `json:"title"`
}

type Service struct {
	repo       *Repository
	users      *user.Repository
	buildings  *building.Service
	properties *property.Service
	pii        *secure.PII
}

func NewService(repo *Repository, users *user.Repository, buildings *building.Service, properties *property.Service, pii *secure.PII) *Service {
	return &Service{repo: repo, users: users, buildings: buildings, properties: properties, pii: pii}
}

func (s *Service) CreateFromText(ctx context.Context, adminID int64, in TextInput) (Result, error) {
	text := strings.TrimSpace(in.Text)
	if text == "" || strings.TrimSpace(in.LandlordPhone) == "" {
		return Result{}, ErrMissingFields
	}
	extracted := parseText(text)
	raw, _ := json.Marshal(extracted)
	enc, _ := s.pii.EncryptString(text)
	jobID, jobUUID, err := s.repo.CreateJob(ctx, adminID, enc, json.RawMessage(raw))
	if err != nil {
		return Result{}, err
	}

	landlord, err := s.users.FindByPhoneHash(ctx, s.pii.HashPhone(in.LandlordPhone))
	if err != nil {
		_ = s.repo.Finish(ctx, jobID, "failed", "unmatched", 0, "", ErrLandlordNotFound.Error())
		return Result{JobID: jobUUID, Status: "failed", Message: ErrLandlordNotFound.Error()}, ErrLandlordNotFound
	}
	if extracted.BuildingName == "" || extracted.AreaSqm <= 0 || extracted.RentYuan <= 0 {
		_ = s.repo.Finish(ctx, jobID, "failed", "auto_matched", landlord.ID, "", ErrMissingFields.Error())
		return Result{JobID: jobUUID, Status: "failed", LandlordUUID: landlord.UUID, Message: ErrMissingFields.Error()}, ErrMissingFields
	}
	b, err := s.buildings.FindByName(ctx, "440300", extracted.BuildingName)
	if err != nil {
		_ = s.repo.Finish(ctx, jobID, "failed", "auto_matched", landlord.ID, "", ErrBuildingNotFound.Error())
		return Result{JobID: jobUUID, Status: "failed", LandlordUUID: landlord.UUID, Message: ErrBuildingNotFound.Error()}, ErrBuildingNotFound
	}

	created, err := s.properties.Create(ctx, landlord.ID, property.CreateInput{
		BuildingUUID: b.UUID,
		AreaSqm:      extracted.AreaSqm,
		RentCents:    int64(extracted.RentYuan * 100),
		Layout:       extracted.Layout,
		Title:        extracted.Title,
		Description:  "管理站文字录入",
	})
	if err != nil {
		_ = s.repo.Finish(ctx, jobID, "failed", "auto_matched", landlord.ID, "", err.Error())
		return Result{JobID: jobUUID, Status: "failed", LandlordUUID: landlord.UUID, Message: err.Error()}, err
	}
	_ = s.repo.Finish(ctx, jobID, "created", "auto_matched", landlord.ID, created.UUID, "")
	return Result{
		JobID:        jobUUID,
		PropertyUUID: created.UUID,
		Status:       created.Status,
		LandlordUUID: landlord.UUID,
		BuildingName: b.Name,
	}, nil
}

func parseText(text string) Extracted {
	out := Extracted{}
	if m := buildingRe.FindStringSubmatch(text); len(m) == 2 {
		out.BuildingName = strings.TrimSpace(m[1])
	}
	if out.BuildingName == "" {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.ContainsAny(line, "：:") {
				out.BuildingName = line
				break
			}
		}
	}
	if m := areaRe.FindStringSubmatch(text); len(m) == 2 {
		out.AreaSqm, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := rentRe.FindStringSubmatch(text); len(m) == 2 {
		out.RentYuan, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := layoutRe.FindStringSubmatch(text); len(m) == 2 {
		out.Layout = strings.TrimSpace(m[1])
	}
	out.Title = fmt.Sprintf("%s · %.0f㎡ · %.0f元/月", out.BuildingName, out.AreaSqm, out.RentYuan)
	return out
}
