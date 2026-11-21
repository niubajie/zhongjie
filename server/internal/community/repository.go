package community

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

func (r *Repository) Suggest(ctx context.Context, cityCode, keyword string, limit int) ([]Community, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	pattern := "%" + keyword + "%"
	rows, err := r.pool.Query(ctx, `
		SELECT id, uuid::text, city_code, district_code, district_name, street_code, street_name,
		       name, aliases, community_type, COALESCE(poi_id, ''), source, review_status,
		       lat::float8, lng::float8
		FROM communities
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
		return nil, fmt.Errorf("suggest communities: %w", err)
	}
	defer rows.Close()

	var out []Community
	for rows.Next() {
		var c Community
		if err := rows.Scan(&c.ID, &c.UUID, &c.CityCode, &c.DistrictCode, &c.DistrictName,
			&c.StreetCode, &c.StreetName, &c.Name, &c.Aliases, &c.Type, &c.POIID,
			&c.Source, &c.ReviewStatus, &c.Lat, &c.Lng); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) FindByName(ctx context.Context, cityCode, name string) (Community, error) {
	var c Community
	err := r.pool.QueryRow(ctx, `
		SELECT id, uuid::text, city_code, district_code, district_name, street_code, street_name,
		       name, aliases, community_type, COALESCE(poi_id, ''), source, review_status,
		       lat::float8, lng::float8
		FROM communities
		WHERE city_code = $1 AND status = 'active' AND deleted_at IS NULL
		  AND review_status = 'approved'
		  AND (name = $2 OR $2 = ANY(aliases) OR name ILIKE '%' || $2 || '%')
		ORDER BY CASE WHEN name = $2 THEN 0 ELSE 1 END, name
		LIMIT 1
	`, cityCode, name).Scan(&c.ID, &c.UUID, &c.CityCode, &c.DistrictCode, &c.DistrictName,
		&c.StreetCode, &c.StreetName, &c.Name, &c.Aliases, &c.Type, &c.POIID,
		&c.Source, &c.ReviewStatus, &c.Lat, &c.Lng)
	if errors.Is(err, pgx.ErrNoRows) {
		return Community{}, ErrNotFound
	}
	if err != nil {
		return Community{}, fmt.Errorf("find community by name: %w", err)
	}
	return c, nil
}

func (r *Repository) GetByUUID(ctx context.Context, uuid string) (Community, error) {
	var c Community
	err := r.pool.QueryRow(ctx, `
		SELECT id, uuid::text, city_code, district_code, district_name, street_code, street_name,
		       name, aliases, community_type, COALESCE(poi_id, ''), source, review_status,
		       lat::float8, lng::float8
		FROM communities
		WHERE uuid = $1 AND status = 'active' AND deleted_at IS NULL
	`, uuid).Scan(&c.ID, &c.UUID, &c.CityCode, &c.DistrictCode, &c.DistrictName,
		&c.StreetCode, &c.StreetName, &c.Name, &c.Aliases, &c.Type, &c.POIID,
		&c.Source, &c.ReviewStatus, &c.Lat, &c.Lng)
	if errors.Is(err, pgx.ErrNoRows) {
		return Community{}, ErrNotFound
	}
	if err != nil {
		return Community{}, fmt.Errorf("get community: %w", err)
	}
	return c, nil
}

func (r *Repository) Create(ctx context.Context, adminID int64, in CreateInput, source string) (Community, error) {
	var c Community
	err := r.pool.QueryRow(ctx, `
		INSERT INTO communities (
			city_code, district_code, district_name, street_code, street_name,
			name, aliases, community_type, poi_id, source, lat, lng
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), $10, $11, $12)
		RETURNING id, uuid::text, city_code, district_code, district_name, street_code, street_name,
		          name, aliases, community_type, COALESCE(poi_id, ''), source, review_status,
		          lat::float8, lng::float8
	`, in.CityCode, in.DistrictCode, in.DistrictName, in.StreetCode, in.StreetName,
		in.Name, in.Aliases, in.Type, in.POIID, source, in.Lat, in.Lng,
	).Scan(&c.ID, &c.UUID, &c.CityCode, &c.DistrictCode, &c.DistrictName,
		&c.StreetCode, &c.StreetName, &c.Name, &c.Aliases, &c.Type, &c.POIID,
		&c.Source, &c.ReviewStatus, &c.Lat, &c.Lng)
	if err != nil {
		return Community{}, fmt.Errorf("create community: %w", err)
	}
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_type, actor_id, action, target_type, target_id)
		VALUES ('admin', $1, 'community_create', 'community', $2)
	`, adminID, c.UUID); err != nil {
		return Community{}, fmt.Errorf("write community audit: %w", err)
	}
	return c, nil
}
