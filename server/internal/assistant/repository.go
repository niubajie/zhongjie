package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSessionNotFound = errors.New("assistant session not found")

type Candidate struct {
	UUID         string
	Title        string
	DistrictName string
	StreetName   string
	AreaSqm      float64
	RentCents    int64
	Layout       string
	BuildingType string
	Lat          float64
	Lng          float64
	ImageCount   int
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateSession(ctx context.Context) (string, error) {
	var uuid string
	if err := r.pool.QueryRow(ctx, `
		INSERT INTO assistant_sessions DEFAULT VALUES RETURNING uuid::text
	`).Scan(&uuid); err != nil {
		return "", fmt.Errorf("create assistant session: %w", err)
	}
	return uuid, nil
}

func (r *Repository) GetSession(ctx context.Context, uuid string) ([]byte, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT slots_json FROM assistant_sessions
		WHERE uuid = $1 AND expires_at > now()
	`, uuid).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get assistant session: %w", err)
	}
	return raw, nil
}

func (r *Repository) UpdateSession(ctx context.Context, uuid string, slots map[string]any, intent string) error {
	raw, err := json.Marshal(slots)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE assistant_sessions
		SET slots_json = $2::jsonb, last_intent = $3, updated_at = now()
		WHERE uuid = $1
	`, uuid, string(raw), intent)
	if err != nil {
		return fmt.Errorf("update assistant session: %w", err)
	}
	return nil
}

func (r *Repository) Candidates(ctx context.Context, cityCode string) ([]Candidate, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.uuid::text, p.title_public, p.district_name, p.street_name,
		       p.area_sqm::float8, p.rent_cents, COALESCE(p.layout, ''), p.building_type,
		       p.lat::float8, p.lng::float8,
		       (SELECT COUNT(*)::int FROM property_images pi
		        WHERE pi.property_id = p.id AND pi.moderation_status = 'approved')
		FROM properties p
		WHERE p.city_code = $1 AND p.status = 'active' AND p.deleted_at IS NULL
		ORDER BY p.last_confirmed_at DESC NULLS LAST, p.published_at DESC NULLS LAST
		LIMIT 500
	`, cityCode)
	if err != nil {
		return nil, fmt.Errorf("assistant candidates: %w", err)
	}
	defer rows.Close()
	out := make([]Candidate, 0, 64)
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.UUID, &c.Title, &c.DistrictName, &c.StreetName,
			&c.AreaSqm, &c.RentCents, &c.Layout, &c.BuildingType, &c.Lat, &c.Lng, &c.ImageCount); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) PropertySummary(ctx context.Context, propertyUUID string) (Candidate, error) {
	var c Candidate
	err := r.pool.QueryRow(ctx, `
		SELECT p.uuid::text, p.title_public, p.district_name, p.street_name,
		       p.area_sqm::float8, p.rent_cents, COALESCE(p.layout, ''), p.building_type,
		       p.lat::float8, p.lng::float8,
		       (SELECT COUNT(*)::int FROM property_images pi
		        WHERE pi.property_id = p.id AND pi.moderation_status = 'approved')
		FROM properties p
		WHERE p.uuid = $1 AND p.status = 'active' AND p.deleted_at IS NULL
	`, propertyUUID).Scan(&c.UUID, &c.Title, &c.DistrictName, &c.StreetName,
		&c.AreaSqm, &c.RentCents, &c.Layout, &c.BuildingType, &c.Lat, &c.Lng, &c.ImageCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return Candidate{}, ErrSessionNotFound
	}
	if err != nil {
		return Candidate{}, fmt.Errorf("property summary: %w", err)
	}
	return c, nil
}

func (r *Repository) Images(ctx context.Context, propertyUUID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT pi.object_key
		FROM property_images pi
		JOIN properties p ON p.id = pi.property_id
		WHERE p.uuid = $1 AND pi.moderation_status = 'approved'
		ORDER BY pi.sort, pi.id
		LIMIT 12
	`, propertyUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

func (r *Repository) Videos(ctx context.Context, propertyUUID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT pm.object_key
		FROM property_media pm
		JOIN properties p ON p.id = pm.property_id
		WHERE p.uuid = $1 AND pm.media_type = 'video' AND pm.moderation_status = 'approved'
		ORDER BY pm.sort, pm.id
		LIMIT 3
	`, propertyUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

func (r *Repository) Location(ctx context.Context, propertyUUID string) (float64, float64, error) {
	var lat, lng float64
	err := r.pool.QueryRow(ctx, `
		SELECT lat::float8, lng::float8 FROM properties
		WHERE uuid = $1 AND status = 'active' AND deleted_at IS NULL
	`, propertyUUID).Scan(&lat, &lng)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, ErrSessionNotFound
	}
	return lat, lng, err
}

func (r *Repository) SaveRecommendation(ctx context.Context, sessionUUID string, filters, result []byte) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO recommendation_runs (session_uuid, filters_json, result_json)
		VALUES ($1, $2::jsonb, $3::jsonb)
	`, sessionUUID, string(filters), string(result))
	if err != nil {
		return fmt.Errorf("save recommendation: %w", err)
	}
	return nil
}

func (r *Repository) SaveNarration(ctx context.Context, propertyUUID string, segments []byte) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO narration_scripts (property_id, version, segments_json)
		SELECT id, 1, $2::jsonb FROM properties WHERE uuid = $1
		ON CONFLICT (property_id, version) DO UPDATE SET segments_json = EXCLUDED.segments_json
	`, propertyUUID, string(segments))
	if err != nil {
		return fmt.Errorf("save narration: %w", err)
	}
	return nil
}
