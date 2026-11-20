package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"housing/internal/auth"
	"housing/internal/secure"
)

var (
	ErrInvalidSurname = errors.New("姓氏需为 1–10 个字符")
	ErrInvalidRole    = errors.New("角色只能是 landlord 或 tenant")
	ErrInvalidPhone   = errors.New("手机号格式不正确")
)

type Service struct {
	repo   *Repository
	pii    *secure.PII
	tokens *auth.Manager
}

func NewService(repo *Repository, pii *secure.PII, tokens *auth.Manager) *Service {
	return &Service{repo: repo, pii: pii, tokens: tokens}
}

func (s *Service) LoginByOpenID(ctx context.Context, openid string) (AuthResult, error) {
	if strings.TrimSpace(openid) == "" {
		return AuthResult{}, errors.New("openid is empty")
	}
	u, err := s.repo.FindByOpenID(ctx, openid)
	if errors.Is(err, ErrNotFound) {
		u, err = s.repo.CreateByOpenID(ctx, openid)
	}
	if err != nil {
		return AuthResult{}, err
	}
	return s.issue(u)
}

func (s *Service) Get(ctx context.Context, id int64) (User, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) CompleteProfile(ctx context.Context, id int64, surname, role string) (User, error) {
	surname = strings.TrimSpace(surname)
	if n := utf8.RuneCountInString(surname); n == 0 || n > 10 {
		return User{}, ErrInvalidSurname
	}
	if role != "landlord" && role != "tenant" {
		return User{}, ErrInvalidRole
	}
	return s.repo.UpdateProfile(ctx, id, surname, role)
}

func (s *Service) SetPhone(ctx context.Context, id int64, rawPhone string) (User, error) {
	phone := secure.NormalizePhone(rawPhone)
	if len(phone) != 11 || !strings.HasPrefix(phone, "1") {
		return User{}, ErrInvalidPhone
	}
	enc, err := s.pii.EncryptString(phone)
	if err != nil {
		return User{}, fmt.Errorf("encrypt phone: %w", err)
	}
	hash := s.pii.HashPhone(phone)
	mask := secure.MaskPhone(phone)
	return s.repo.UpdatePhone(ctx, id, enc, hash, mask)
}

func (s *Service) issue(u User) (AuthResult, error) {
	token, exp, err := s.tokens.Issue(u.ID, u.UUID, u.Role)
	if err != nil {
		return AuthResult{}, fmt.Errorf("issue token: %w", err)
	}
	return AuthResult{
		Token:            token,
		ExpiresAt:        exp,
		ProfileCompleted: u.ProfileCompleted,
		User:             u.Public(),
	}, nil
}
