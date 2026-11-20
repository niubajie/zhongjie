-- 0002_location_names.sql
-- Human-readable district/street names. Codes stay the stable key; names are
-- denormalized on properties so map/detail queries do not need a join.

ALTER TABLE buildings
    ADD COLUMN IF NOT EXISTS district_name text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS street_name text NOT NULL DEFAULT '';

ALTER TABLE properties
    ADD COLUMN IF NOT EXISTS district_name text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS street_name text NOT NULL DEFAULT '';

UPDATE buildings SET district_name = '南山区', street_name = '粤海街道'
WHERE city_code = '440300' AND district_code = '440305' AND street_code = '440305006';
UPDATE buildings SET district_name = '南山区', street_name = '粤海街道'
WHERE city_code = '440300' AND district_code = '440305' AND street_code = '440305007';
UPDATE buildings SET district_name = '福田区', street_name = '福田街道'
WHERE city_code = '440300' AND district_code = '440304' AND street_code = '440304003';

UPDATE properties p
SET district_name = b.district_name,
    street_name = b.street_name
FROM buildings b
WHERE p.building_id = b.id
  AND (p.district_name = '' OR p.street_name = '');
