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
	fields     map[string]string
}

type Result struct {
	Sent          int `json:"sent"`
	Failed        int `json:"failed"`
	NotConfigured int `json:"notConfigured"`
}

func NewService(repo *Repository, pii *secure.PII, wx *wechat.Client, templateID, page, fieldSpec string) *Service {
	return &Service{repo: repo, pii: pii, wx: wx, templateID: templateID, page: page, fields: parseFields(fieldSpec)}
}

func parseFields(spec string) map[string]string {
	out := map[string]string{
		"thing1": "reminder",
		"thing2": "title",
		"thing3": "location",
	}
	for _, pair := range strings.Split(spec, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			continue
		}
		out[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return out
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
		values := map[string]string{
			"reminder": "租客联系提醒",
			"title":    payload.Title,
			"district": payload.District,
			"street":   payload.Street,
			"location": strings.TrimSpace(payload.District + " " + payload.Street),
		}
		data := make(map[string]string, len(s.fields))
		for field, source := range s.fields {
			data[field] = clip(values[source], 20)
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
