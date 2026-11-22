package prospect

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"housing/internal/secure"
)

// Service 承载业务判断；采集 / 评分 / 联系 / 跟进在后续批次扩展。
// 批次 1 只提供信息源与线索的基础读写，不启用采集。
type Service struct {
	repo   *Repository
	pii    *secure.PII
	halted atomic.Bool
}

func NewService(repo *Repository, pii *secure.PII) *Service {
	s := &Service{repo: repo, pii: pii}
	// 启动时加载全局停机状态；失败不阻塞启动（默认不停机，主链路零影响）。
	if v, err := repo.GetSetting(context.Background(), SettingKeyGlobalHalt); err == nil {
		s.halted.Store(parseBoolSetting(v))
	}
	return s
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

// ---- 批次 2：全局停机开关 ----

// IsHalted 返回全局停机状态，供采集器 / 联系流程热读（无锁）。
func (s *Service) IsHalted() bool {
	return s.halted.Load()
}

func (s *Service) GetKillSwitch() KillSwitchState {
	return KillSwitchState{Halted: s.halted.Load()}
}

// SetHalted 切换全局停机：先持久化，成功后更新内存并写审计。
func (s *Service) SetHalted(ctx context.Context, adminID int64, halted bool) error {
	val := "false"
	if halted {
		val = "true"
	}
	if err := s.repo.UpsertSetting(ctx, SettingKeyGlobalHalt, val, adminID); err != nil {
		return err
	}
	s.halted.Store(halted)
	action := "prospect.killswitch.disable"
	if halted {
		action = "prospect.killswitch.enable"
	}
	s.repo.Audit(ctx, adminID, action, "prospect_setting", SettingKeyGlobalHalt)
	return nil
}

// ---- 批次 2：信息源更新与软删除 ----

func (s *Service) UpdateSource(ctx context.Context, adminID int64, uuid string, in UpdateSourceInput) error {
	if in.Name == "" {
		return fmt.Errorf("%w: 名称不能为空", ErrInvalidSourceInput)
	}
	if !validSourceKinds[in.Kind] {
		return fmt.Errorf("%w: 类型不合法", ErrInvalidSourceInput)
	}
	if err := s.repo.UpdateSource(ctx, uuid, in); err != nil {
		return err
	}
	s.repo.Audit(ctx, adminID, "prospect.source.update", "lead_source", uuid)
	return nil
}

func (s *Service) DeleteSource(ctx context.Context, adminID int64, uuid string) error {
	if err := s.repo.SoftDeleteSource(ctx, uuid); err != nil {
		return err
	}
	s.repo.Audit(ctx, adminID, "prospect.source.delete", "lead_source", uuid)
	return nil
}

// ---- 批次 5：人工复核 + 首条联系 ----

// ListReviewQueue 返回待复核队列（已评分达标的 status=new 线索）。
func (s *Service) ListReviewQueue(ctx context.Context, leadType string, limit int) ([]ProspectLeadVO, error) {
	if leadType != "" && leadType != LeadTypeTenant && leadType != LeadTypeLandlord {
		return nil, fmt.Errorf("%w: %s", ErrInvalidStatus, leadType)
	}
	leads, err := s.repo.ListReviewQueue(ctx, leadType, ReviewScoreThreshold, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ProspectLeadVO, 0, len(leads))
	for _, l := range leads {
		out = append(out, toLeadVO(l))
	}
	return out, nil
}

// GetLeadDetail 读取线索详情：解密原文并写入审计日志（合规：每次读取必须留痕）。
func (s *Service) GetLeadDetail(ctx context.Context, adminID int64, uuid string) (ProspectLeadDetailVO, error) {
	lead, err := s.repo.GetLead(ctx, uuid)
	if err != nil {
		return ProspectLeadDetailVO{}, err
	}
	followUps, err := s.repo.ListFollowUps(ctx, lead.ID)
	if err != nil {
		return ProspectLeadDetailVO{}, err
	}
	// 先写审计再解密，保证任何一次原文读取都可追溯。
	s.repo.Audit(ctx, adminID, "prospect.lead.view", "prospect_lead", uuid)

	detail := ProspectLeadDetailVO{
		ProspectLeadVO: toLeadVO(lead),
		FollowUps:      make([]FollowUpVO, 0, len(followUps)),
	}
	if s.pii != nil {
		if raw, err := s.pii.DecryptString(lead.RawTextEnc); err == nil {
			detail.RawText = raw
		}
	}
	for _, f := range followUps {
		content := ""
		if s.pii != nil {
			if c, err := s.pii.DecryptString(f.ContentEnc); err == nil {
				content = c
			}
		}
		detail.FollowUps = append(detail.FollowUps, toFollowUpVO(f, content))
	}
	return detail, nil
}

// ReviewLead 人工复核：approve → reviewed，reject → rejected，均写审计。
func (s *Service) ReviewLead(ctx context.Context, adminID int64, uuid, decision string) (ProspectLeadVO, error) {
	target, err := reviewTargetStatus(decision)
	if err != nil {
		return ProspectLeadVO{}, err
	}
	lead, err := s.repo.GetLead(ctx, uuid)
	if err != nil {
		return ProspectLeadVO{}, err
	}
	if !CanTransition(lead.Status, target) {
		return ProspectLeadVO{}, fmt.Errorf("%w: %s → %s", ErrInvalidTransition, lead.Status, target)
	}
	if err := s.repo.TransitionLeadStatus(ctx, uuid, lead.Status, target); err != nil {
		return ProspectLeadVO{}, err
	}
	action := "prospect.lead.approve"
	if target == StatusRejected {
		action = "prospect.lead.reject"
	}
	s.repo.Audit(ctx, adminID, action, "prospect_lead", uuid)

	updated, err := s.repo.GetLead(ctx, uuid)
	if err != nil {
		return ProspectLeadVO{}, err
	}
	return toLeadVO(updated), nil
}

// ContactLead 人工发起首条联系：内容由操作员填写，服务端不会自动发送。
// 前置条件：已复核（reviewed）、未停机、未命中拒收名单。
func (s *Service) ContactLead(ctx context.Context, adminID int64, uuid string, in ContactLeadInput) (FollowUpVO, error) {
	if err := validateContactInput(in); err != nil {
		return FollowUpVO{}, err
	}
	if s.IsHalted() {
		return FollowUpVO{}, ErrHalted
	}
	lead, err := s.repo.GetLead(ctx, uuid)
	if err != nil {
		return FollowUpVO{}, err
	}
	if !CanTransition(lead.Status, StatusContacted) {
		return FollowUpVO{}, fmt.Errorf("%w: 当前状态 %s 不能发起首条联系", ErrInvalidTransition, lead.Status)
	}
	if len(lead.ContactHash) > 0 {
		optedOut, err := s.repo.OptOutExists(ctx, lead.ContactHash, lead.LeadType)
		if err != nil {
			return FollowUpVO{}, err
		}
		if optedOut {
			return FollowUpVO{}, ErrOptOutExists
		}
	}
	if s.pii == nil {
		return FollowUpVO{}, errors.New("联系内容加密未初始化")
	}
	content := strings.TrimSpace(in.Content)
	contentEnc, err := s.pii.EncryptString(content)
	if err != nil {
		return FollowUpVO{}, err
	}
	auditID := s.repo.Audit(ctx, adminID, "prospect.lead.contact", "prospect_lead", uuid)
	followUp, err := s.repo.ContactLead(ctx, lead.ID, in.Channel, contentEnc, auditID)
	if err != nil {
		return FollowUpVO{}, err
	}
	return toFollowUpVO(followUp, content), nil
}

// ---- 批次 6：状态流转与拒收 ----

// TransitionLead 手动流转线索状态。reviewed / contacted / opt_out 各有专用接口，
// 通用流转只接受 following_up / converted / published / cold / lost / rejected / ignored。
func (s *Service) TransitionLead(ctx context.Context, adminID int64, uuid, target string) (ProspectLeadVO, error) {
	if target == StatusReviewed || target == StatusContacted || target == StatusOptOut {
		return ProspectLeadVO{}, fmt.Errorf("%w: %s 请使用对应专用接口", ErrInvalidStatus, target)
	}
	if !ValidAdminTargetStatus(target) {
		return ProspectLeadVO{}, fmt.Errorf("%w: %s", ErrInvalidStatus, target)
	}
	lead, err := s.repo.GetLead(ctx, uuid)
	if err != nil {
		return ProspectLeadVO{}, err
	}
	if !CanTransition(lead.Status, target) {
		return ProspectLeadVO{}, fmt.Errorf("%w: %s → %s", ErrInvalidTransition, lead.Status, target)
	}
	if err := s.repo.TransitionLeadStatus(ctx, uuid, lead.Status, target); err != nil {
		return ProspectLeadVO{}, err
	}
	s.repo.Audit(ctx, adminID, "prospect.lead.transition."+target, "prospect_lead", uuid)

	updated, err := s.repo.GetLead(ctx, uuid)
	if err != nil {
		return ProspectLeadVO{}, err
	}
	return toLeadVO(updated), nil
}

// OptOutLead 人工拒收：写入全局拒收名单（按 contact_hash 跨源生效），并把线索置为 opt_out。
// 先写名单再改状态：即使状态并发变化，也保证后续联系被拦截。
func (s *Service) OptOutLead(ctx context.Context, adminID int64, uuid, reason string) (ProspectLeadVO, error) {
	lead, err := s.repo.GetLead(ctx, uuid)
	if err != nil {
		return ProspectLeadVO{}, err
	}
	if !CanTransition(lead.Status, StatusOptOut) {
		return ProspectLeadVO{}, fmt.Errorf("%w: 当前状态 %s 不能拒收", ErrInvalidTransition, lead.Status)
	}
	if len(lead.ContactHash) > 0 {
		if err := s.repo.CreateOptOut(ctx, lead.ContactHash, lead.LeadType, strings.TrimSpace(reason)); err != nil {
			return ProspectLeadVO{}, err
		}
	}
	if err := s.repo.TransitionLeadStatus(ctx, uuid, lead.Status, StatusOptOut); err != nil {
		return ProspectLeadVO{}, err
	}
	s.repo.Audit(ctx, adminID, "prospect.lead.optout", "prospect_lead", uuid)

	updated, err := s.repo.GetLead(ctx, uuid)
	if err != nil {
		return ProspectLeadVO{}, err
	}
	return toLeadVO(updated), nil
}

// ListOptOuts 拒收名单列表。
func (s *Service) ListOptOuts(ctx context.Context, leadType string, limit int) ([]OptOutVO, error) {
	if leadType != "" && leadType != LeadTypeTenant && leadType != LeadTypeLandlord {
		return nil, fmt.Errorf("%w: %s", ErrInvalidStatus, leadType)
	}
	rows, err := s.repo.ListOptOuts(ctx, leadType, limit)
	if err != nil {
		return nil, err
	}
	out := make([]OptOutVO, 0, len(rows))
	for _, o := range rows {
		out = append(out, OptOutVO{
			ID:         o.ID,
			LeadType:   o.LeadType,
			Reason:     o.Reason,
			OptedOutAt: o.OptedOutAt,
		})
	}
	return out, nil
}

// reviewTargetStatus 把复核结论映射为目标状态。
func reviewTargetStatus(decision string) (string, error) {
	switch decision {
	case ReviewDecisionApprove:
		return StatusReviewed, nil
	case ReviewDecisionReject:
		return StatusRejected, nil
	}
	return "", fmt.Errorf("%w: %s", ErrInvalidReview, decision)
}

// validateContactInput 联系渠道与内容校验（首条联系必须人工填写内容）。
func validateContactInput(in ContactLeadInput) error {
	switch in.Channel {
	case ChannelInApp, ChannelSMS, ChannelWeChat, ChannelPhone:
	default:
		return fmt.Errorf("%w: 联系渠道不合法", ErrInvalidContact)
	}
	if strings.TrimSpace(in.Content) == "" {
		return fmt.Errorf("%w: 联系内容不能为空", ErrInvalidContact)
	}
	return nil
}

func toFollowUpVO(f FollowUp, content string) FollowUpVO {
	return FollowUpVO{
		ID:        f.ID,
		Channel:   f.Channel,
		Direction: f.Direction,
		Status:    f.Status,
		Content:   content,
		SentAt:    f.SentAt,
		CreatedAt: f.CreatedAt,
	}
}

// parseBoolSetting 解析配置值为布尔（供单测覆盖）。
func parseBoolSetting(v string) bool {
	return v == "true"
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
		Stale:             l.StaleAt != "",
		StaleAt:           l.StaleAt,
		CreatedAt:         l.CreatedAt,
		UpdatedAt:         l.UpdatedAt,
	}
}
