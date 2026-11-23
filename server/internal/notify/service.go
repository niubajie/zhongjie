package notify

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"housing/internal/secure"
)

// DefaultMaxAttempts 单条外发记录的最大尝试次数（含首次）。
const DefaultMaxAttempts = 5

// Store 是通知外发所需的存储能力（由 Repository 实现，测试可替换）。
type Store interface {
	Due(ctx context.Context, limit int) ([]Record, error)
	OpenID(ctx context.Context, userID int64) (string, error)
	MarkSent(ctx context.Context, id int64) error
	MarkRetry(ctx context.Context, id int64, message string, maxAttempts int) (string, error)
	MarkTerminal(ctx context.Context, id int64, status, message string) error
	List(ctx context.Context, status string, limit int) ([]OutboxRecord, error)
	Retry(ctx context.Context, id int64) error
	Audit(ctx context.Context, adminID int64, action, targetType, targetID string) int64
}

// Sender 是微信模板消息发送能力（由 wechat.Client 实现，测试可替换）。
type Sender interface {
	Configured() bool
	SendSubscribeMessage(ctx context.Context, openID, templateID, page string, data map[string]string) error
}

type Service struct {
	repo        Store
	pii         *secure.PII
	wx          Sender
	templateID  string
	page        string
	fields      map[string]string
	maxAttempts int
}

type Result struct {
	Sent          int `json:"sent"`
	Retrying      int `json:"retrying"`
	Dead          int `json:"dead"`
	NotConfigured int `json:"notConfigured"`
}

func NewService(repo Store, pii *secure.PII, wx Sender, templateID, page, fieldSpec string, maxAttempts int) *Service {
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	return &Service{
		repo: repo, pii: pii, wx: wx, templateID: templateID, page: page,
		fields: parseFields(fieldSpec), maxAttempts: maxAttempts,
	}
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
			_ = s.repo.MarkTerminal(ctx, rec.ID, StatusNotConfigured,
				"WECHAT_SUBSCRIBE_TEMPLATE_ID or WeChat credentials not configured")
			result.NotConfigured++
			continue
		}
		plain, err := s.pii.DecryptString(rec.PayloadEnc)
		if err != nil {
			_ = s.repo.MarkTerminal(ctx, rec.ID, StatusFailed, "decrypt payload failed")
			result.Dead++
			continue
		}
		var payload struct {
			Reminder string `json:"reminder"`
			Title    string `json:"title"`
			District string `json:"district"`
			Street   string `json:"street"`
		}
		_ = json.Unmarshal([]byte(plain), &payload)
		openid, err := s.repo.OpenID(ctx, rec.RecipientID)
		if err != nil || openid == "" {
			_ = s.repo.MarkTerminal(ctx, rec.ID, StatusFailed, "recipient openid missing")
			result.Dead++
			continue
		}
		reminderLabel := strings.TrimSpace(payload.Reminder)
		if reminderLabel == "" {
			reminderLabel = "租客联系提醒"
		}
		values := map[string]string{
			"reminder": reminderLabel,
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
			status, markErr := s.repo.MarkRetry(ctx, rec.ID, err.Error(), s.maxAttempts)
			if markErr != nil {
				return result, markErr
			}
			if status == StatusFailed {
				result.Dead++
			} else {
				result.Retrying++
			}
			continue
		}
		if err := s.repo.MarkSent(ctx, rec.ID); err != nil {
			return result, err
		}
		result.Sent++
	}
	return result, nil
}

// OutboxRecordVO 是外发记录的管理站视图。
type OutboxRecordVO struct {
	ID          int64  `json:"id"`
	EventType   string `json:"eventType"`
	RecipientID int64  `json:"recipientUserId"`
	Status      string `json:"status"`
	Attempts    int    `json:"attempts"`
	LastError   string `json:"lastError,omitempty"`
	NextRetryAt string `json:"nextRetryAt,omitempty"`
	SentAt      string `json:"sentAt,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type OutboxListVO struct {
	Items []OutboxRecordVO `json:"items"`
}

// ListOutbox 查询外发记录，status 为空表示全部。
func (s *Service) ListOutbox(ctx context.Context, status string, limit int) (OutboxListVO, error) {
	rows, err := s.repo.List(ctx, status, limit)
	if err != nil {
		return OutboxListVO{}, err
	}
	out := OutboxListVO{Items: make([]OutboxRecordVO, 0, len(rows))}
	for _, rec := range rows {
		out.Items = append(out.Items, OutboxRecordVO{
			ID:          rec.ID,
			EventType:   rec.EventType,
			RecipientID: rec.RecipientID,
			Status:      rec.Status,
			Attempts:    rec.Attempts,
			LastError:   rec.LastError,
			NextRetryAt: rec.NextRetryAt,
			SentAt:      rec.SentAt,
			CreatedAt:   rec.CreatedAt,
			UpdatedAt:   rec.UpdatedAt,
		})
	}
	return out, nil
}

// RetryOutbox 手动重试一条失败或未配置的外发记录，并写审计日志。
func (s *Service) RetryOutbox(ctx context.Context, adminID, id int64) error {
	if err := s.repo.Retry(ctx, id); err != nil {
		return err
	}
	s.repo.Audit(ctx, adminID, "prospect.notification_outbox.retry", "notification_outbox", strconv.FormatInt(id, 10))
	return nil
}

func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= max {
		return s
	}
	return string([]rune(s)[:max])
}
