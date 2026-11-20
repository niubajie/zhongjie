package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	"housing/internal/db"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://housing:housing@localhost:55432/housing?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback(ctx)

	var landlordID int64
	err = tx.QueryRow(ctx, `SELECT id FROM users WHERE openid = 'dev-landlord-1' AND deleted_at IS NULL`).Scan(&landlordID)
	if err != nil {
		err = tx.QueryRow(ctx, `
			INSERT INTO users (openid, surname, role, status)
			VALUES ('dev-landlord-1', '王', 'landlord', 'active')
			RETURNING id
		`).Scan(&landlordID)
		if err != nil {
			log.Fatal(err)
		}
	}

	buildings := []struct {
		name, district, districtName, street, streetName, typ string
		lat, lng                                              float64
	}{
		{"深圳湾科技生态园", "440305", "南山区", "440305006", "粤海街道", "industrial_park", 22.5172, 113.9496},
		{"科兴科学园", "440305", "南山区", "440305006", "粤海街道", "industrial_park", 22.5405, 113.9528},
		{"腾讯滨海大厦", "440305", "南山区", "440305006", "粤海街道", "office_building", 22.5230, 113.9430},
		{"深圳国际创新中心", "440304", "福田区", "440304003", "福田街道", "office_building", 22.5401, 114.0579},
	}
	buildingIDs := make(map[string]int64, len(buildings))
	for _, b := range buildings {
		var id int64
		err := tx.QueryRow(ctx, `SELECT id FROM buildings WHERE city_code='440300' AND name=$1 AND deleted_at IS NULL LIMIT 1`, b.name).Scan(&id)
		if err != nil {
			err = tx.QueryRow(ctx, `
				INSERT INTO buildings (city_code, district_code, district_name, street_code, street_name, name, building_type, lat, lng, source, review_status)
				VALUES ('440300', $1, $2, $3, $4, $5, $6, $7, $8, 'manual', 'approved')
				RETURNING id
			`, b.district, b.districtName, b.street, b.streetName, b.name, b.typ, b.lat, b.lng).Scan(&id)
			if err != nil {
				log.Fatal(err)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				UPDATE buildings
				SET district_code=$2, district_name=$3, street_code=$4, street_name=$5
				WHERE id=$1
			`, id, b.district, b.districtName, b.street, b.streetName); err != nil {
				log.Fatal(err)
			}
		}
		buildingIDs[b.name] = id
	}

	listings := []struct {
		building, title  string
		district, street string
		area             float64
		rentCents        int64
		layout, floor    string
		orientation      string
	}{
		{"深圳湾科技生态园", "南山科技园 精装 120㎡ 办公", "440305", "440305006", 120, 1680000, "3+1", "12/30", "南"},
		{"深圳湾科技生态园", "生态园高层 80㎡ 采光好", "440305", "440305006", 80, 1080000, "2+1", "18/30", "东南"},
		{"科兴科学园", "科兴科学园 150㎡ 带家私", "440305", "440305007", 150, 2250000, "4+1", "8/28", "西南"},
		{"腾讯滨海大厦", "滨海大厦旁 200㎡ 海景办公", "440305", "440305006", 200, 3600000, "5+2", "25/40", "南"},
		{"深圳国际创新中心", "福田中心区 90㎡ 写字楼", "440304", "440304003", 90, 1800000, "2+1", "15/35", "东"},
	}

	for i, l := range listings {
		var propertyID int64
		err := tx.QueryRow(ctx, `
			SELECT id FROM properties
			WHERE landlord_id = $1 AND title_public = $2 AND deleted_at IS NULL
			LIMIT 1
		`, landlordID, l.title).Scan(&propertyID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `
				INSERT INTO properties (
					landlord_id, city_code, district_code, district_name, street_code, street_name,
					building_id, building_name, building_type,
					lat, lng, area_sqm, rent_cents, layout, floor, orientation,
					title_public, description_public, status, auto_publish_eligible, source_type,
					published_at, last_confirmed_at
				)
				SELECT $1, '440300', $2, b.district_name, $3, b.street_name,
				       $4, $5, b.building_type,
				       b.lat, b.lng, $6, $7, $8, $9, $10,
				       $11, $12, 'active', true, 'miniapp', now(), now()
				FROM buildings b WHERE b.id = $4
				RETURNING id
			`, landlordID, l.district, l.street, buildingIDs[l.building], l.building,
				l.area, l.rentCents, l.layout, l.floor, l.orientation,
				l.title, "近地铁，采光好，可随时看房。",
			).Scan(&propertyID)
		}
		if err != nil {
			log.Fatal(err)
		}
		// Seed image keys point at placeholder assets; the mini program renders a
		// local gradient cover until the COS signing endpoint is wired.
		for j := 0; j < 3; j++ {
			key := fmt.Sprintf("seed/p%d_%d.jpg", i+1, j+1)
			if _, err := tx.Exec(ctx, `
				INSERT INTO property_images (property_id, object_key, width, height, sort, moderation_status)
				SELECT $1, $2, 1200, 800, $3, 'approved'
				WHERE NOT EXISTS (
					SELECT 1 FROM property_images WHERE property_id = $1 AND object_key = $2
				)
			`, propertyID, key, j); err != nil {
				log.Fatal(err)
			}
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE properties p
		SET district_code = b.district_code,
		    district_name = b.district_name,
		    street_code = b.street_code,
		    street_name = b.street_name
		FROM buildings b
		WHERE p.building_id = b.id
	`); err != nil {
		log.Fatal(err)
	}

	if err := tx.Commit(ctx); err != nil {
		log.Fatal(err)
	}
	log.Printf("seed complete: landlord=%d buildings=%d listings=%d", landlordID, len(buildings), len(listings))
}
