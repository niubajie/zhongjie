package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound   = errors.New("user not found")
	ErrPhoneTaken = errors.New("phone already bound")
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const userColumns = `
	id, uuid::text,
	COALESCE(openid, ''), COALESCE(unionid, ''),
	COALESCE(phone_mask, ''), COALESCE(surname, ''),
	COALESCE(role, ''), status,
	(phone_hash IS NOT NULL AND COALESCE(surname, '') <> '' AND COALESCE(role, '') <> '') AS profile_completed,
	created_at
`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(
		&u.ID, &u.UUID, &u.OpenID, &u.UnionID,
		&u.PhoneMask, &u.Surname, &u.Role, &u.Status,
		&u.ProfileCompleted, &u.CreatedAt,
	)
	if err != nil {
		return User{}, err
	}
	return u, nil
}

func (r *Repository) FindByID(ctx context.Context, id int64) (User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+userColumns+`
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	u, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("find user by id: %w", err)
	}
	return u, nil
}

func (r *Repository) FindByUUID(ctx context.Context, uuid string) (User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+userColumns+`
		FROM users
		WHERE uuid = $1 AND deleted_at IS NULL
	`, uuid)
	u, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("find user by uuid: %w", err)
	}
	return u, nil
}

func (r *Repository) FindByPhoneHash(ctx context.Context, hash []byte) (User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+userColumns+`
		FROM users
		WHERE phone_hash = $1 AND deleted_at IS NULL
	`, hash)
	u, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("find user by phone hash: %w", err)
	}
	return u, nil
}

func (r *Repository) FindByOpenID(ctx context.Context, openid string) (User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+userColumns+`
		FROM users
		WHERE openid = $1 AND deleted_at IS NULL
	`, openid)
	u, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("find user by openid: %w", err)
	}
	return u, nil
}

func (r *Repository) CreateByOpenID(ctx context.Context, openid string) (User, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO users (openid)
		VALUES ($1)
		RETURNING `+userColumns+`
	`, openid)
	u, err := scanUser(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return r.FindByOpenID(ctx, openid)
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

func (r *Repository) UpdateProfile(ctx context.Context, id int64, surname, role string) (User, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE users
		SET surname = $2, role = $3
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING `+userColumns+`
	`, id, surname, role)
	u, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("update profile: %w", err)
	}
	return u, nil
}

func (r *Repository) UpdatePhone(ctx context.Context, id int64, enc, hash []byte, mask string) (User, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE users
		SET phone_enc = $2, phone_hash = $3, phone_mask = $4
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING `+userColumns+`
	`, id, enc, hash, mask)
	u, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return User{}, ErrPhoneTaken
		}
		return User{}, fmt.Errorf("update phone: %w", err)
	}
	return u, nil
}
