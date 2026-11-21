-- 0008_communities_media_usage.sql
-- Residential community dictionary, media intake and model usage records.

CREATE TABLE IF NOT EXISTS communities (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    uuid            uuid NOT NULL DEFAULT uuidv7(),
    city_code       text NOT NULL,
    district_code   text NOT NULL,
    district_name   text NOT NULL,
    street_code     text NOT NULL,
    street_name     text NOT NULL,
    name            text NOT NULL,
    aliases         text[] NOT NULL DEFAULT '{}',
    community_type  text NOT NULL DEFAULT 'residential'
                    CHECK (community_type IN ('residential', 'apartment')),
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

CREATE UNIQUE INDEX IF NOT EXISTS communities_uuid_uq ON communities (uuid);
CREATE UNIQUE INDEX IF NOT EXISTS communities_city_name_uq
    ON communities (city_code, name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS communities_city_district_idx ON communities (city_code, district_code);
CREATE INDEX IF NOT EXISTS communities_lat_lng_idx ON communities (lat, lng);

CREATE TRIGGER communities_set_updated_at BEFORE UPDATE ON communities
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE properties
    ADD COLUMN IF NOT EXISTS community_id bigint REFERENCES communities (id),
    ADD COLUMN IF NOT EXISTS community_name text;

ALTER TABLE properties DROP CONSTRAINT IF EXISTS properties_building_type_check;
ALTER TABLE properties ADD CONSTRAINT properties_building_type_check
    CHECK (building_type IN ('residential', 'apartment', 'industrial_park', 'office_building'));

CREATE INDEX IF NOT EXISTS properties_community_idx ON properties (community_id);

ALTER TABLE admin_intake_jobs
    ADD COLUMN IF NOT EXISTS media_keys text[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS transcript_enc bytea,
    ADD COLUMN IF NOT EXISTS ocr_text_enc bytea,
    ADD COLUMN IF NOT EXISTS match_confidence numeric(4, 3) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS error_code text,
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE admin_intake_jobs DROP CONSTRAINT IF EXISTS admin_intake_jobs_status_check;
ALTER TABLE admin_intake_jobs ADD CONSTRAINT admin_intake_jobs_status_check
    CHECK (status IN ('pending', 'processing', 'ready', 'created', 'failed', 'not_configured'));

CREATE TABLE IF NOT EXISTS media_assets (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    intake_id       bigint NOT NULL REFERENCES admin_intake_jobs (id) ON DELETE CASCADE,
    kind            text NOT NULL CHECK (kind IN ('image', 'video', 'keyframe')),
    object_key      text NOT NULL,
    duration_ms     integer,
    width           integer,
    height          integer,
    keyframe_ms     integer,
    transcript_enc  bytea,
    ocr_text_enc    bytea,
    moderation_status text NOT NULL DEFAULT 'pending'
                    CHECK (moderation_status IN ('pending', 'approved', 'rejected')),
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS media_assets_intake_idx ON media_assets (intake_id, kind);

CREATE TABLE IF NOT EXISTS usage_records (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    uuid            uuid NOT NULL DEFAULT uuidv7(),
    provider        text NOT NULL,
    model           text NOT NULL,
    kind            text NOT NULL CHECK (kind IN ('asr', 'tts', 'ocr', 'llm')),
    input_units     bigint NOT NULL DEFAULT 0,
    output_units    bigint NOT NULL DEFAULT 0,
    cost_cents      bigint NOT NULL DEFAULT 0,
    trace_id        text,
    status          text NOT NULL DEFAULT 'ok' CHECK (status IN ('ok', 'failed')),
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS usage_records_created_idx ON usage_records (created_at DESC);
CREATE INDEX IF NOT EXISTS usage_records_kind_idx ON usage_records (kind, created_at DESC);
