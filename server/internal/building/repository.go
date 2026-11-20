package building

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

func (r *Repository) Suggest(ctx context.Context, cityCode, keyword string, limit int) ([]Building, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	pattern := "%" + keyword + "%"
	rows, err := r.pool.Query(ctx, `
		SELECT id, uuid::text, city_code, name, district_code, district_name, street_code, street_name, building_type,
		       lat::float8, lng::float8, review_status
		FROM buildings
		WHERE city_code = $1
		  AND status = 'active'
		  AND deleted_at IS NULL
		  AND review_status = 'approved'
		  AND ($2 = '' OR name ILIKE $3 OR EXISTS (
		      SELECT 1 FROM unnest(aliases) AS alias WHERE alias ILIKE $3
		  ))
		ORDER BY
		  CASE WHEN name = $2 THEN 0 WHEN name ILIKE $2 THEN 1 ELSE 2 END,
		  name ASC
		LIMIT $4
	`, cityCode, keyword, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("suggest buildings: %w", err)
	}
	defer rows.Close()

	var out []Building
	for rows.Next() {
		var b Building
		if err := rows.Scan(&b.ID, &b.UUID, &b.CityCode, &b.Name, &b.DistrictCode, &b.DistrictName, &b.StreetCode, &b.StreetName,
			&b.BuildingType, &b.Lat, &b.Lng, &b.ReviewStatus); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *Repository) GetByUUID(ctx context.Context, uuid string) (Building, error) {
	var b Building
	err := r.pool.QueryRow(ctx, `
		SELECT id, uuid::text, city_code, name, district_code, district_name, street_code, street_name, building_type,
		       lat::float8, lng::float8, review_status
		FROM buildings
		WHERE uuid = $1 AND status = 'active' AND deleted_at IS NULL
	`, uuid).Scan(&b.ID, &b.UUID, &b.CityCode, &b.Name, &b.DistrictCode, &b.DistrictName, &b.StreetCode, &b.StreetName,
		&b.BuildingType, &b.Lat, &b.Lng, &b.ReviewStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return Building{}, ErrNotFound
	}
	if err != nil {
		return Building{}, fmt.Errorf("get building: %w", err)
	}
	return b, nil
}
