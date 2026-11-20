package property

import (
	"context"
	"fmt"

	"housing/internal/building"
)

func (r *Repository) Create(ctx context.Context, landlordID int64, b building.Building, in CreateInput, status, pendingReason string) (Created, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Created{}, fmt.Errorf("begin create property: %w", err)
	}
	defer tx.Rollback(ctx)

	published := status == "active"
	var created Created
	err = tx.QueryRow(ctx, `
		INSERT INTO properties (
			landlord_id, city_code, district_code, district_name, street_code, street_name,
			building_id, building_name, building_type,
			lat, lng, area_sqm, rent_cents, layout, floor, orientation,
			title_public, description_public, status, pending_reason,
			auto_publish_eligible, source_type, published_at, last_confirmed_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9,
			$10, $11, $12, $13, $14, $15, $16,
			$17, $18, $19, $20,
			true, 'miniapp',
			CASE WHEN $21 THEN now() ELSE NULL END,
			CASE WHEN $21 THEN now() ELSE NULL END
		)
		RETURNING uuid::text, status, COALESCE(pending_reason, '')
	`, landlordID, b.CityCode, b.DistrictCode, b.DistrictName, b.StreetCode, b.StreetName,
		b.ID, b.Name, b.BuildingType,
		b.Lat, b.Lng, in.AreaSqm, in.RentCents, in.Layout, in.Floor, in.Orientation,
		in.Title, in.Description, status, pendingReason, published,
	).Scan(&created.UUID, &created.Status, &created.PendingReason)
	if err != nil {
		return Created{}, fmt.Errorf("insert property: %w", err)
	}

	for i, key := range in.ImageKeys {
		if _, err := tx.Exec(ctx, `
			INSERT INTO property_images (property_id, object_key, sort, moderation_status)
			SELECT id, $2, $3, 'pending'
			FROM properties WHERE uuid = $1
		`, created.UUID, key, i); err != nil {
			return Created{}, fmt.Errorf("insert property image: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Created{}, fmt.Errorf("commit create property: %w", err)
	}
	return created, nil
}
