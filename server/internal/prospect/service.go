package prospect

import (
	"context"
	"errors"
	"fmt"
)

// Service 承载业务判断；采集 / 评分 / 联系 / 跟进在后续批次扩展。
// 批次 1 只提供信息源与线索的基础读写，不启用采集。
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

var validSourceKinds = map[string]bool{
	"self_owned":         true,
	"public_api":         true,
	"third_party_scrape": true,
}

func (s *Service) CreateSource(ctx context.Context, adminID int64, in CreateSourceInput) (LeadSourceVO, error) {
	if in.Name == "" {
		return LeadSourceVO{}, errors.New("信息源名称不能为空")
	}
	if !validSourceKinds[in.Kind] {
		return LeadSourceVO{}, errors.New("信息源类型不合法")
	}
	_, uuid, err := s.repo.CreateSource(ctx, in)
	if err != nil {
		return LeadSourceVO{}, err
	}
	s.repo.Audit(ctx, adminID, "prospect.source.create", "lead_source", uuid)
	src, err := s.repo.GetSource(ctx, uuid)
	if err != nil {
		return LeadSourceVO{}, err
	}
	return toSourceVO(src), nil
}

func (s *Service) ListSources(ctx context.Context) ([]LeadSourceVO, error) {
	sources, err := s.repo.ListSources(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]LeadSourceVO, 0, len(sources))
	for _, src := range sources {
		out = append(out, toSourceVO(src))
	}
	return out, nil
}

func (s *Service) SetSourceEnabled(ctx context.Context, adminID int64, uuid string, enabled bool) error {
	if err := s.repo.SetSourceEnabled(ctx, uuid, enabled); err != nil {
		return err
	}
	action := "prospect.source.disable"
	if enabled {
		action = "prospect.source.enable"
	}
	s.repo.Audit(ctx, adminID, action, "lead_source", uuid)
	return nil
}

func (s *Service) ListLeads(ctx context.Context, leadType, status string, limit int) ([]ProspectLeadVO, error) {
	if leadType != "" && leadType != LeadTypeTenant && leadType != LeadTypeLandlord {
		return nil, fmt.Errorf("%w: %s", ErrInvalidStatus, leadType)
	}
	if status != "" && !ValidStatus(status) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidStatus, status)
	}
	leads, err := s.repo.ListLeads(ctx, leadType, status, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ProspectLeadVO, 0, len(leads))
	for _, l := range leads {
		out = append(out, toLeadVO(l))
	}
	return out, nil
}

func (s *Service) GetLead(ctx context.Context, uuid string) (ProspectLeadVO, error) {
	l, err := s.repo.GetLead(ctx, uuid)
	if err != nil {
		return ProspectLeadVO{}, err
	}
	return toLeadVO(l), nil
}

func toSourceVO(s LeadSource) LeadSourceVO {
	return LeadSourceVO{
		UUID:        s.UUID,
		Name:        s.Name,
		Kind:        s.Kind,
		BaseURL:     s.BaseURL,
		RobotsOK:    s.RobotsOK,
		TermsOK:     s.TermsOK,
		RateLimitMs: s.RateLimitMs,
		Enabled:     s.Enabled,
		ReviewNote:  s.ReviewNote,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}

func toLeadVO(l ProspectLead) ProspectLeadVO {
	return ProspectLeadVO{
		UUID:              l.UUID,
		SourceName:        l.SourceName,
		LeadType:          l.LeadType,
		Platform:          l.Platform,
		ContactChannel:    l.ContactChannel,
		CityCode:          l.CityCode,
		DistrictCode:      l.DistrictCode,
		BudgetCents:       l.BudgetCents,
		AreaSqm:           l.AreaSqm,
		Layout:            l.Layout,
		IntentScore:       l.IntentScore,
		IntentStatus:      l.IntentStatus,
		Status:            l.Status,
		MatchedPropertyID: l.MatchedPropertyID,
		CreatedAt:         l.CreatedAt,
		UpdatedAt:         l.UpdatedAt,
	}
}
