package freshness

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Respond(ctx context.Context, userID int64, uuid, status string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var propertyID, landlordID int64
	err = tx.QueryRow(ctx, `
		SELECT id, landlord_id FROM properties
		WHERE uuid = $1 AND deleted_at IS NULL
	`, uuid).Scan(&propertyID, &landlordID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("find freshness property: %w", err)
	}
	if landlordID != userID {
		return "", ErrNotOwner
	}

	var next string
	err = tx.QueryRow(ctx, `
		UPDATE properties
		SET status = $2,
		    last_confirmed_at = CASE WHEN $2 = 'active' THEN now() ELSE last_confirmed_at END
		WHERE id = $1
		RETURNING status
	`, propertyID, status).Scan(&next)
	if err != nil {
		return "", fmt.Errorf("update freshness: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO freshness_checks (property_id, sent_at, responded_at, response, status)
		VALUES ($1, now(), now(), $2, 'responded')
	`, propertyID, status); err != nil {
		return "", fmt.Errorf("record freshness response: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return next, nil
}

func (r *Repository) CreateDueChecks(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO freshness_checks (property_id, status)
		SELECT p.id, 'pending'
		FROM properties p
		WHERE p.status = 'active'
		  AND p.deleted_at IS NULL
		  AND COALESCE(p.last_confirmed_at, p.published_at) < now() - interval '30 days'
		  AND NOT EXISTS (
		      SELECT 1 FROM freshness_checks fc
		      WHERE fc.property_id = p.id
		        AND fc.status = 'pending'
		        AND fc.created_at > now() - interval '7 days'
		  )
	`)
	if err != nil {
		return 0, fmt.Errorf("create freshness checks: %w", err)
	}
	return tag.RowsAffected(), nil
}
