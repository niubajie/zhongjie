package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pquerna/otp/totp"

	"housing/internal/auth"
	"housing/internal/secure"
)

var (
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrDisabled           = errors.New("账号已禁用")
	ErrTOTPRequired       = errors.New("需要动态验证码")
	ErrTOTPInvalid        = errors.New("动态验证码不正确")
	ErrInvalidStatus      = errors.New("房源状态不正确")
)

type Service struct {
	repo   *Repository
	pii    *secure.PII
	tokens *auth.Manager
}

func NewService(repo *Repository, pii *secure.PII, tokens *auth.Manager) *Service {
	return &Service{repo: repo, pii: pii, tokens: tokens}
}

func (s *Service) Login(ctx context.Context, username, password, code string) (LoginResult, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return LoginResult{}, ErrInvalidCredentials
	}
	a, err := s.repo.FindByUsername(ctx, username)
	if errors.Is(err, ErrNotFound) {
		return LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, err
	}
	ok, err := VerifyPassword(password, a.PasswordHash)
	if err != nil || !ok {
		return LoginResult{}, ErrInvalidCredentials
	}
	if a.Status != "active" {
		return LoginResult{}, ErrDisabled
	}
	if a.TOTPEnabled {
		if strings.TrimSpace(code) == "" {
			return LoginResult{}, ErrTOTPRequired
		}
		secret, err := s.pii.DecryptString(a.TOTPSecretEnc)
		if err != nil {
			return LoginResult{}, fmt.Errorf("decrypt totp secret: %w", err)
		}
		if !totp.Validate(strings.TrimSpace(code), secret) {
			return LoginResult{}, ErrTOTPInvalid
		}
	}

	token, exp, err := s.tokens.Issue(a.ID, a.UUID, a.Role)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue admin token: %w", err)
	}
	if err := s.repo.UpdateLastLogin(ctx, a.ID); err != nil {
		return LoginResult{}, err
	}
	if err := s.repo.AuditLogin(ctx, a.ID, a.Username); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: token, ExpiresAt: exp, Admin: a.Public()}, nil
}

func (s *Service) Get(ctx context.Context, id int64) (Admin, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) SetupTOTP(ctx context.Context, id int64) (string, string, error) {
	a, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return "", "", err
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "房屋中介管理站",
		AccountName: a.Username,
	})
	if err != nil {
		return "", "", fmt.Errorf("generate totp: %w", err)
	}
	enc, err := s.pii.EncryptString(key.Secret())
	if err != nil {
		return "", "", err
	}
	if err := s.repo.SetTOTPSecret(ctx, id, enc); err != nil {
		return "", "", err
	}
	s.audit(ctx, id, "admin_totp_setup", "admin_user", a.UUID)
	return key.Secret(), key.URL(), nil
}

func (s *Service) EnableTOTP(ctx context.Context, id int64, code string) error {
	a, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if len(a.TOTPSecretEnc) == 0 {
		return ErrTOTPRequired
	}
	secret, err := s.pii.DecryptString(a.TOTPSecretEnc)
	if err != nil {
		return err
	}
	if !totp.Validate(strings.TrimSpace(code), secret) {
		return ErrTOTPInvalid
	}
	if err := s.repo.SetTOTPEnabled(ctx, id, true); err != nil {
		return err
	}
	s.audit(ctx, id, "admin_totp_enable", "admin_user", a.UUID)
	return nil
}

func (s *Service) DisableTOTP(ctx context.Context, id int64, code string) error {
	a, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if len(a.TOTPSecretEnc) == 0 {
		return nil
	}
	secret, err := s.pii.DecryptString(a.TOTPSecretEnc)
	if err != nil {
		return err
	}
	if !totp.Validate(strings.TrimSpace(code), secret) {
		return ErrTOTPInvalid
	}
	if err := s.repo.SetTOTPEnabled(ctx, id, false); err != nil {
		return err
	}
	s.audit(ctx, id, "admin_totp_disable", "admin_user", a.UUID)
	return nil
}

func (s *Service) ModerateProperty(ctx context.Context, adminID int64, uuid, action, reason string) (string, error) {
	action = strings.TrimSpace(action)
	if action != "approve" && action != "offline" {
		return "", ErrInvalidStatus
	}
	status, err := s.repo.ModerateProperty(ctx, strings.TrimSpace(uuid), action, clip(reason, 200))
	if err != nil {
		return "", err
	}
	s.audit(ctx, adminID, "property_moderate", "property", uuid)
	return status, nil
}

func (s *Service) audit(ctx context.Context, adminID int64, action, targetType, targetID string) {
	// Audit failures must not break a successful primary action; they are
	// surfaced in logs by the repository caller in a later iteration.
	_ = s.repo.Audit(ctx, adminID, action, targetType, targetID)
}

func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

func (s *Service) Stats(ctx context.Context) (Stats, error) {
	return s.repo.Stats(ctx)
}

func (s *Service) ListProperties(ctx context.Context, status string, limit, offset int) ([]PropertyRow, error) {
	status = strings.TrimSpace(status)
	switch status {
	case "", "draft", "pending", "active", "rented", "offline", "stale":
	default:
		return nil, ErrInvalidStatus
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListProperties(ctx, status, limit, offset)
}
