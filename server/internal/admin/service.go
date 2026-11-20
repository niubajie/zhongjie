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
