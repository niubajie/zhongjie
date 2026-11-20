package notify

import (
	"context"
	"encoding/json"
	"strings"

	"housing/internal/secure"
	"housing/internal/wechat"
)

type Service struct {
	repo       *Repository
	pii        *secure.PII
	wx         *wechat.Client
	templateID string
	page       string
}

type Result struct {
	Sent          int `json:"sent"`
	Failed        int `json:"failed"`
	NotConfigured int `json:"notConfigured"`
}

func NewService(repo *Repository, pii *secure.PII, wx *wechat.Client, templateID, page string) *Service {
	return &Service{repo: repo, pii: pii, wx: wx, templateID: templateID, page: page}
}

func (s *Service) Process(ctx context.Context, limit int) (Result, error) {
	records, err := s.repo.Due(ctx, limit)
	if err != nil {
		return Result{}, err
	}
	result := Result{}
	for _, rec := range records {
		if s.templateID == "" || !s.wx.Configured() {
			_ = s.repo.Mark(ctx, rec.ID, "not_configured", "WECHAT_SUBSCRIBE_TEMPLATE_ID or WeChat credentials not configured")
			result.NotConfigured++
			continue
		}
		plain, err := s.pii.DecryptString(rec.PayloadEnc)
		if err != nil {
			_ = s.repo.Mark(ctx, rec.ID, "failed", "decrypt payload failed")
			result.Failed++
			continue
		}
		var payload struct {
			Title    string `json:"title"`
			District string `json:"district"`
			Street   string `json:"street"`
		}
		_ = json.Unmarshal([]byte(plain), &payload)
		openid, err := s.repo.OpenID(ctx, rec.RecipientID)
		if err != nil || openid == "" {
			_ = s.repo.Mark(ctx, rec.ID, "failed", "recipient openid missing")
			result.Failed++
			continue
		}
		// Template field names must match the template approved in the WeChat
		// console. These are the common "thing" fields for a contact reminder.
		data := map[string]string{
			"thing1": clip("租客联系提醒", 20),
			"thing2": clip(payload.Title, 20),
			"thing3": clip(payload.District+" "+payload.Street, 20),
		}
		if err := s.wx.SendSubscribeMessage(ctx, openid, s.templateID, s.page, data); err != nil {
			_ = s.repo.Mark(ctx, rec.ID, "failed", err.Error())
			result.Failed++
			continue
		}
		_ = s.repo.Mark(ctx, rec.ID, "sent", "")
		result.Sent++
	}
	return result, nil
}

func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= max {
		return s
	}
	return string([]rune(s)[:max])
}
