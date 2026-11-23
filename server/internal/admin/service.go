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
	ErrTOTPSetupRequired  = errors.New("请先生成 TOTP 密钥")
	ErrLoginLocked        = errors.New("登录失败次数过多，请 15 分钟后再试")
	ErrInvalidStatus      = errors.New("房源状态不正确")
	ErrInvalidBuilding    = errors.New("楼宇名称、区域、类型或坐标不正确")
)

type Service struct {
	repo     *Repository
	pii      *secure.PII
	tokens   *auth.Manager
	throttle *loginThrottle
}

func NewService(repo *Repository, pii *secure.PII, tokens *auth.Manager) *Service {
	return &Service{repo: repo, pii: pii, tokens: tokens, throttle: newLoginThrottle()}
}

func (s *Service) Login(ctx context.Context, username, password, code string) (LoginResult, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return LoginResult{}, ErrInvalidCredentials
	}
	if s.throttle.locked(username) > 0 {
		return LoginResult{}, ErrLoginLocked
	}
	a, err := s.repo.FindByUsername(ctx, username)
	if errors.Is(err, ErrNotFound) {
		// 不存在的账号不计数，避免随机用户名撑大内存；真实账号仍受锁定保护。
		return LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, err
	}
	ok, err := VerifyPassword(password, a.PasswordHash)
	if err != nil || !ok {
		s.throttle.fail(username)
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
			s.throttle.fail(username)
			return LoginResult{}, ErrTOTPInvalid
		}
	}
	s.throttle.reset(username)

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

func (s *Service) SetupTOTP(ctx context.Context, id int64, code string) (string, string, error) {
	a, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return "", "", err
	}
	// 已开启 TOTP 时，重新绑定必须先验证当前验证码，避免仅凭登录态替换密钥。
	if a.TOTPEnabled {
		if strings.TrimSpace(code) == "" {
			return "", "", ErrTOTPRequired
		}
		current, err := s.pii.DecryptString(a.TOTPSecretEnc)
		if err != nil {
			return "", "", err
		}
		if !totp.Validate(strings.TrimSpace(code), current) {
			return "", "", ErrTOTPInvalid
		}
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
		return ErrTOTPSetupRequired
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
	if !a.TOTPEnabled {
		return nil
	}
	secret, err := s.pii.DecryptString(a.TOTPSecretEnc)
	if err != nil {
		return err
	}
	if !totp.Validate(strings.TrimSpace(code), secret) {
		return ErrTOTPInvalid
	}
	if err := s.repo.ClearTOTP(ctx, id); err != nil {
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

func (s *Service) ListImports(ctx context.Context, limit int) ([]ImportJobRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.ListImports(ctx, limit)
}

func (s *Service) CreateBuilding(ctx context.Context, adminID int64, in CreateBuildingInput) (BuildingRow, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.DistrictCode = strings.TrimSpace(in.DistrictCode)
	in.DistrictName = strings.TrimSpace(in.DistrictName)
	in.StreetCode = strings.TrimSpace(in.StreetCode)
	in.StreetName = strings.TrimSpace(in.StreetName)
	if in.Name == "" || in.DistrictCode == "" || in.DistrictName == "" ||
		in.StreetCode == "" || in.StreetName == "" {
		return BuildingRow{}, ErrInvalidBuilding
	}
	if in.BuildingType != "industrial_park" && in.BuildingType != "office_building" {
		return BuildingRow{}, ErrInvalidBuilding
	}
	if in.Lat < 3 || in.Lat > 54 || in.Lng < 73 || in.Lng > 136 {
		return BuildingRow{}, ErrInvalidBuilding
	}
	row, err := s.repo.CreateBuilding(ctx, in)
	if err != nil {
		return BuildingRow{}, err
	}
	s.audit(ctx, adminID, "building_create", "building", row.UUID)
	return row, nil
}

func (s *Service) ListBuildings(ctx context.Context, limit int) ([]BuildingRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	return s.repo.ListBuildings(ctx, limit)
}

func (s *Service) ListContacts(ctx context.Context, limit int) ([]ContactRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.ListContacts(ctx, limit)
}

func (s *Service) GetContact(ctx context.Context, uuid string) (ContactDetail, error) {
	return s.repo.GetContact(ctx, strings.TrimSpace(uuid))
}

func (s *Service) ListAuditLogs(ctx context.Context, limit int) ([]AuditLogRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	return s.repo.ListAuditLogs(ctx, limit)
}

func (s *Service) ListFreshness(ctx context.Context, limit int) ([]FreshnessRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.ListFreshness(ctx, limit)
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
