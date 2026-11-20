-- 0007_admin_intake.sql
-- Management-site rental intake jobs (text first; media can be queued later).

CREATE TABLE IF NOT EXISTS admin_intake_jobs (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    uuid            uuid NOT NULL DEFAULT uuidv7(),
    admin_id        bigint NOT NULL REFERENCES admin_users (id),
    input_type      text NOT NULL CHECK (input_type IN ('text', 'image', 'video')),
    raw_text_enc    bytea,
    media_key       text,
    extracted_json  jsonb NOT NULL DEFAULT '{}'::jsonb,
    landlord_id     bigint REFERENCES users (id),
    match_status    text NOT NULL DEFAULT 'unmatched'
                    CHECK (match_status IN ('auto_matched', 'candidates', 'unmatched', 'manual_matched')),
    property_uuid   uuid,
    status          text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'created', 'failed', 'not_configured')),
    error_message   text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    completed_at    timestamptz
);

CREATE INDEX IF NOT EXISTS admin_intake_jobs_admin_idx ON admin_intake_jobs (admin_id, created_at DESC);
CREATE INDEX IF NOT EXISTS admin_intake_jobs_status_idx ON admin_intake_jobs (status, created_at DESC);
