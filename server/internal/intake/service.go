package intake

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"housing/internal/community"
	"housing/internal/modelgw"
	"housing/internal/property"
	"housing/internal/secure"
	"housing/internal/storage"
	"housing/internal/user"
)

var (
	communityRe = regexp.MustCompile(`(?:小区|楼盘|社区|项目|地址)\s*[:：]\s*(.+)`)
	areaRe      = regexp.MustCompile(`面积\s*[:：]?\s*(\d+(?:\.\d+)?)`)
	rentRe      = regexp.MustCompile(`(?:租金|月租|价格)\s*[:：]?\s*(\d+(?:\.\d+)?)`)
	layoutRe    = regexp.MustCompile(`(?:户型|房型)\s*[:：]?\s*([0-9+]+室?[0-9]*厅?)`)
	mobileRe    = regexp.MustCompile(`1[3-9]\d{9}`)
)

type Extracted struct {
	CommunityName string  `json:"communityName"`
	Phone         string  `json:"phone,omitempty"`
	AreaSqm       float64 `json:"areaSqm"`
	RentYuan      float64 `json:"rentYuan"`
	Layout        string  `json:"layout"`
	Title         string  `json:"title"`
}

type Service struct {
	repo        *Repository
	users       *user.Repository
	communities *community.Service
	properties  *property.Service
	pii         *secure.PII
	gw          *modelgw.Gateway
	media       *storage.Service
}

func NewService(repo *Repository, users *user.Repository, communities *community.Service, properties *property.Service, pii *secure.PII, gw *modelgw.Gateway, media *storage.Service) *Service {
	return &Service{repo: repo, users: users, communities: communities, properties: properties, pii: pii, gw: gw, media: media}
}

func (s *Service) List(ctx context.Context, status string, limit int) ([]JobSummary, error) {
	status = strings.TrimSpace(status)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.ListJobs(ctx, status, limit)
}

func (s *Service) CreateFromText(ctx context.Context, adminID int64, in TextInput) (Result, error) {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return Result{}, ErrMissingFields
	}
	extracted := parseText(text)
	landlordPhone := strings.TrimSpace(in.LandlordPhone)
	if landlordPhone == "" {
		landlordPhone = extracted.Phone
	}
	if landlordPhone == "" {
		return Result{}, ErrMissingFields
	}
	stored := extracted
	stored.Phone = "" // the raw phone stays only inside the encrypted text blob
	raw, _ := json.Marshal(stored)
	enc, _ := s.pii.EncryptString(text)
	jobID, jobUUID, err := s.repo.CreateJob(ctx, adminID, enc, json.RawMessage(raw))
	if err != nil {
		return Result{}, err
	}

	landlord, err := s.users.FindByPhoneHash(ctx, s.pii.HashPhone(landlordPhone))
	if err != nil {
		_ = s.repo.Finish(ctx, jobID, "failed", "unmatched", 0, "", ErrLandlordNotFound.Error())
		return Result{JobID: jobUUID, Status: "failed", Message: ErrLandlordNotFound.Error()}, ErrLandlordNotFound
	}
	if extracted.CommunityName == "" || extracted.AreaSqm <= 0 || extracted.RentYuan <= 0 {
		_ = s.repo.Finish(ctx, jobID, "failed", "auto_matched", landlord.ID, "", ErrMissingFields.Error())
		return Result{JobID: jobUUID, Status: "failed", LandlordUUID: landlord.UUID, Message: ErrMissingFields.Error()}, ErrMissingFields
	}
	c, err := s.communities.FindByName(ctx, "440300", extracted.CommunityName)
	if err != nil {
		_ = s.repo.Finish(ctx, jobID, "failed", "auto_matched", landlord.ID, "", ErrCommunityNotFound.Error())
		return Result{JobID: jobUUID, Status: "failed", LandlordUUID: landlord.UUID, Message: ErrCommunityNotFound.Error()}, ErrCommunityNotFound
	}

	created, err := s.properties.Create(ctx, landlord.ID, property.CreateInput{
		CommunityUUID: c.UUID,
		AreaSqm:       extracted.AreaSqm,
		RentCents:     int64(extracted.RentYuan * 100),
		Layout:        extracted.Layout,
		Title:         extracted.Title,
		Description:   "管理站文字录入",
	})
	if err != nil {
		_ = s.repo.Finish(ctx, jobID, "failed", "auto_matched", landlord.ID, "", err.Error())
		return Result{JobID: jobUUID, Status: "failed", LandlordUUID: landlord.UUID, Message: err.Error()}, err
	}
	_ = s.repo.Finish(ctx, jobID, "created", "auto_matched", landlord.ID, created.UUID, "")
	return Result{
		JobID:         jobUUID,
		PropertyUUID:  created.UUID,
		Status:        created.Status,
		LandlordUUID:  landlord.UUID,
		CommunityName: c.Name,
	}, nil
}

func parseText(text string) Extracted {
	out := Extracted{}
	if m := communityRe.FindStringSubmatch(text); len(m) == 2 {
		out.CommunityName = strings.TrimSpace(m[1])
	}
	if out.CommunityName == "" {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.ContainsAny(line, "：:") {
				out.CommunityName = line
				break
			}
		}
	}
	if m := mobileRe.FindString(text); m != "" {
		out.Phone = m
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
	out.Title = fmt.Sprintf("%s · %.0f㎡ · %.0f元/月", out.CommunityName, out.AreaSqm, out.RentYuan)
	return out
}
