-- 0001_init.sql
-- Core identity, building dictionary, listings and contact requests.
-- Money is stored as integer cents. Public listing location is district/street only.

CREATE TABLE IF NOT EXISTS users (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    uuid          uuid NOT NULL DEFAULT uuidv7(),
    openid        text,
    unionid       text,
    phone_enc     bytea,
    phone_hash    bytea,
    phone_mask    text,
    surname       text,
    role          text CHECK (role IN ('landlord', 'tenant')),
    status        text NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active', 'unclaimed', 'disabled', 'deleted')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    deleted_at    timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS users_openid_uq
    ON users (openid) WHERE openid IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS users_phone_hash_uq
    ON users (phone_hash) WHERE phone_hash IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS users_created_at_idx ON users (created_at);

CREATE TABLE IF NOT EXISTS buildings (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    uuid            uuid NOT NULL DEFAULT uuidv7(),
    city_code       text NOT NULL,
    district_code   text NOT NULL,
    street_code     text NOT NULL,
    name            text NOT NULL,
    aliases         text[] NOT NULL DEFAULT '{}',
    building_type   text NOT NULL CHECK (building_type IN ('industrial_park', 'office_building')),
    poi_id          text,
    source          text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'lbs', 'import')),
    review_status   text NOT NULL DEFAULT 'approved'
                    CHECK (review_status IN ('pending', 'approved', 'rejected')),
    lat             numeric(9, 6) NOT NULL,
    lng             numeric(9, 6) NOT NULL,
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);

CREATE INDEX IF NOT EXISTS buildings_city_name_idx ON buildings (city_code, name);
CREATE INDEX IF NOT EXISTS buildings_city_district_idx ON buildings (city_code, district_code);
CREATE INDEX IF NOT EXISTS buildings_lat_lng_idx ON buildings (lat, lng);

CREATE TABLE IF NOT EXISTS properties (
    id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    uuid                uuid NOT NULL DEFAULT uuidv7(),
    landlord_id         bigint NOT NULL REFERENCES users (id),
    city_code           text NOT NULL,
    district_code       text NOT NULL,
    street_code         text NOT NULL,
    building_id         bigint REFERENCES buildings (id),
    building_name       text NOT NULL,
    building_type       text NOT NULL CHECK (building_type IN ('industrial_park', 'office_building')),
    address_enc         bytea,
    lat                 numeric(9, 6) NOT NULL,
    lng                 numeric(9, 6) NOT NULL,
    area_sqm            numeric(6, 2) NOT NULL CHECK (area_sqm > 0),
    rent_cents          bigint NOT NULL CHECK (rent_cents > 0),
    layout              text,
    floor               text,
    orientation         text,
    title_public        text NOT NULL,
    description_public  text,
    raw_text_enc        bytea,
    status              text NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('draft', 'pending', 'active', 'rented', 'offline', 'stale')),
    pending_reason      text,
    auto_publish_eligible boolean NOT NULL DEFAULT false,
    source_type         text NOT NULL DEFAULT 'miniapp'
                        CHECK (source_type IN ('miniapp', 'docx', 'xlsx', 'screenshot', 'admin_text', 'admin_image', 'admin_video')),
    published_at        timestamptz,
    expires_at          timestamptz,
    last_confirmed_at   timestamptz,
    import_job_id       bigint,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    deleted_at          timestamptz
);

CREATE INDEX IF NOT EXISTS properties_landlord_status_idx ON properties (landlord_id, status);
CREATE INDEX IF NOT EXISTS properties_city_status_idx ON properties (city_code, status, published_at DESC);
CREATE INDEX IF NOT EXISTS properties_building_idx ON properties (building_id);
CREATE INDEX IF NOT EXISTS properties_lat_lng_idx ON properties (lat, lng);
CREATE INDEX IF NOT EXISTS properties_bbox_idx ON properties (city_code, lat, lng) WHERE status = 'active' AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS property_images (
    id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    property_id         bigint NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    object_key          text NOT NULL,
    width               integer,
    height              integer,
    blurhash            text,
    sort                integer NOT NULL DEFAULT 0,
    moderation_status   text NOT NULL DEFAULT 'pending'
                        CHECK (moderation_status IN ('pending', 'approved', 'rejected')),
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS property_images_property_sort_idx ON property_images (property_id, sort);

CREATE TABLE IF NOT EXISTS contact_requests (
    id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    uuid                uuid NOT NULL DEFAULT uuidv7(),
    tenant_id           bigint NOT NULL REFERENCES users (id),
    landlord_id         bigint NOT NULL REFERENCES users (id),
    property_id         bigint NOT NULL REFERENCES properties (id),
    status              text NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'notified', 'failed', 'closed')),
    note                text,
    consent_snapshot    jsonb NOT NULL DEFAULT '{}'::jsonb,
    ip_hash             bytea,
    ua                  text,
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS contact_requests_tenant_idx ON contact_requests (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS contact_requests_property_idx ON contact_requests (property_id, created_at DESC);

CREATE TABLE IF NOT EXISTS audit_logs (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_type      text NOT NULL,
    actor_id        bigint,
    action          text NOT NULL,
    target_type     text NOT NULL,
    target_id       text,
    ip_hash         bytea,
    ua              text,
    trace_id        text,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS audit_logs_created_at_idx ON audit_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_target_idx ON audit_logs (target_type, target_id);

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS users_set_updated_at ON users;
CREATE TRIGGER users_set_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS buildings_set_updated_at ON buildings;
CREATE TRIGGER buildings_set_updated_at BEFORE UPDATE ON buildings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS properties_set_updated_at ON properties;
CREATE TRIGGER properties_set_updated_at BEFORE UPDATE ON properties
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
