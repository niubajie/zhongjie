package contact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"housing/internal/secure"
)

type Service struct {
	repo *Repository
	pii  *secure.PII
}

func NewService(repo *Repository, pii *secure.PII) *Service {
	return &Service{repo: repo, pii: pii}
}

func (s *Service) Create(ctx context.Context, tenantID int64, in CreateInput) (Reveal, error) {
	if !in.Consent {
		return Reveal{}, ErrConsent
	}
	hasPhone, err := s.repo.TenantHasPhone(ctx, tenantID)
	if err != nil {
		return Reveal{}, err
	}
	if !hasPhone {
		return Reveal{}, ErrPhoneRequired
	}

	target, err := s.repo.FindTarget(ctx, strings.TrimSpace(in.PropertyUUID))
	if err != nil {
		return Reveal{}, err
	}
	recent, err := s.repo.CountRecent(ctx, tenantID, target.PropertyID)
	if err != nil {
		return Reveal{}, err
	}
	if recent > 0 {
		return Reveal{}, ErrAlreadyContacted
	}
	today, err := s.repo.CountToday(ctx, tenantID)
	if err != nil {
		return Reveal{}, err
	}
	if today >= 3 {
		return Reveal{}, ErrDailyLimit
	}

	landlord, err := s.repo.FindLandlord(ctx, target.LandlordID)
	if err != nil {
		return Reveal{}, err
	}

	phone := ""
	if landlord.SharePhone {
		phone, err = s.pii.DecryptString(landlord.PhoneEnc)
		if err != nil {
			return Reveal{}, fmt.Errorf("decrypt landlord phone: %w", err)
		}
	}
	wechatID := ""
	if landlord.ShareWechat {
		wechatID, err = s.pii.DecryptString(landlord.WechatEnc)
		if err != nil {
			return Reveal{}, fmt.Errorf("decrypt landlord wechat: %w", err)
		}
	}

	requestUUID, err := s.repo.Create(ctx, tenantID, target, clip(in.Note, 200))
	if err != nil {
		return Reveal{}, err
	}
	if err := s.repo.Audit(ctx, tenantID, requestUUID); err != nil {
		return Reveal{}, err
	}

	notificationStatus := "not_configured"
	payload, _ := json.Marshal(map[string]any{
		"requestUuid": requestUUID,
		"tenantId":    tenantID,
		"propertyId":  target.PropertyID,
		"title":       target.Title,
		"district":    target.DistrictName,
		"street":      target.StreetName,
	})
	if enc, err := s.pii.EncryptString(string(payload)); err == nil {
		if err := s.repo.EnqueueNotification(ctx, target.LandlordID, enc); err == nil {
			notificationStatus = "queued"
		}
	}

	available := phone != "" || wechatID != ""
	message := "已记录联系请求，通知已进入队列。"
	if notificationStatus != "queued" {
		message = "已记录联系请求；通知服务暂不可用。"
	}
	if !available {
		message = "已记录联系请求；房东暂未授权展示联系方式。"
	}
	return Reveal{
		RequestUUID:        requestUUID,
		Phone:              phone,
		WechatID:           wechatID,
		ContactAvailable:   available,
		NotificationStatus: notificationStatus,
		Message:            message,
	}, nil
}

func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}
