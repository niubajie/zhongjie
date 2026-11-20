package property

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

func (r *Repository) Map(ctx context.Context, q MapQuery) (MapResult, error) {
	result := MapResult{CityCode: q.CityCode, Zoom: q.Zoom, Clusters: []MapCluster{}, Items: []MapItem{}}

	if q.Zoom >= 15 {
		rows, err := r.pool.Query(ctx, `
			SELECT uuid::text, district_code, district_name, street_code, street_name,
			       lat::float8, lng::float8,
			       area_sqm::float8, rent_cents,
			       COALESCE(layout, ''), title_public
			FROM properties
			WHERE city_code = $1
			  AND status = 'active'
			  AND deleted_at IS NULL
			  AND lat BETWEEN $2 AND $3
			  AND lng BETWEEN $4 AND $5
			ORDER BY last_confirmed_at DESC NULLS LAST, published_at DESC NULLS LAST
			LIMIT 300
		`, q.CityCode, q.SWLat, q.NELat, q.SWLng, q.NELng)
		if err != nil {
			return MapResult{}, fmt.Errorf("query map items: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var item MapItem
			if err := rows.Scan(&item.UUID, &item.DistrictCode, &item.DistrictName, &item.StreetCode, &item.StreetName,
				&item.Lat, &item.Lng, &item.AreaSqm, &item.RentCents, &item.Layout, &item.Title); err != nil {
				return MapResult{}, err
			}
			result.Items = append(result.Items, item)
		}
		return result, rows.Err()
	}

	rows, err := r.pool.Query(ctx, `
		SELECT district_code, district_name, street_code, street_name, COUNT(*)::bigint,
		       AVG(lat)::float8, AVG(lng)::float8
		FROM properties
		WHERE city_code = $1
		  AND status = 'active'
		  AND deleted_at IS NULL
		  AND lat BETWEEN $2 AND $3
		  AND lng BETWEEN $4 AND $5
		GROUP BY district_code, district_name, street_code, street_name
		ORDER BY COUNT(*) DESC, street_code ASC
		LIMIT 300
	`, q.CityCode, q.SWLat, q.NELat, q.SWLng, q.NELng)
	if err != nil {
		return MapResult{}, fmt.Errorf("query map clusters: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c MapCluster
		if err := rows.Scan(&c.DistrictCode, &c.DistrictName, &c.StreetCode, &c.StreetName, &c.Count, &c.Lat, &c.Lng); err != nil {
			return MapResult{}, err
		}
		c.ID = "street:" + c.StreetCode
		c.Level = "street"
		result.Clusters = append(result.Clusters, c)
	}
	return result, rows.Err()
}

func (r *Repository) Detail(ctx context.Context, uuid string) (Detail, error) {
	var d Detail
	err := r.pool.QueryRow(ctx, `
		SELECT uuid::text, district_code, district_name, street_code, street_name,
		       area_sqm::float8, rent_cents,
		       COALESCE(layout, ''), COALESCE(floor, ''), COALESCE(orientation, ''),
		       title_public, COALESCE(description_public, ''),
		       building_type,
		       COALESCE(to_char(published_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
		       COALESCE(to_char(last_confirmed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), '')
		FROM properties
		WHERE uuid = $1 AND status = 'active' AND deleted_at IS NULL
	`, uuid).Scan(
		&d.UUID, &d.DistrictCode, &d.DistrictName, &d.StreetCode, &d.StreetName,
		&d.AreaSqm, &d.RentCents,
		&d.Layout, &d.Floor, &d.Orientation,
		&d.Title, &d.Description, &d.BuildingType,
		&d.PublishedAt, &d.LastConfirmedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, fmt.Errorf("query property detail: %w", err)
	}
	d.Images = []Image{}

	rows, err := r.pool.Query(ctx, `
		SELECT pi.object_key, COALESCE(pi.width, 0), COALESCE(pi.height, 0), pi.sort
		FROM property_images pi
		JOIN properties p ON p.id = pi.property_id
		WHERE p.uuid = $1 AND pi.moderation_status = 'approved'
		ORDER BY pi.sort ASC, pi.id ASC
	`, uuid)
	if err != nil {
		return Detail{}, fmt.Errorf("query property images: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var img Image
		if err := rows.Scan(&img.ObjectKey, &img.Width, &img.Height, &img.Sort); err != nil {
			return Detail{}, err
		}
		d.Images = append(d.Images, img)
	}
	return d, rows.Err()
}
