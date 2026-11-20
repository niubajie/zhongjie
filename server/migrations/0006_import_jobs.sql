-- 0006_import_jobs.sql
-- Landlord document import jobs and parsed drafts.

CREATE TABLE IF NOT EXISTS import_jobs (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    uuid            uuid NOT NULL DEFAULT uuidv7(),
    landlord_id     bigint NOT NULL REFERENCES users (id),
    file_name       text NOT NULL,
    file_type       text NOT NULL CHECK (file_type IN ('docx', 'xlsx', 'png', 'jpg', 'jpeg')),
    status          text NOT NULL DEFAULT 'processing'
                    CHECK (status IN ('processing', 'completed', 'failed')),
    total_rows      integer NOT NULL DEFAULT 0,
    success_rows    integer NOT NULL DEFAULT 0,
    error_rows      integer NOT NULL DEFAULT 0,
    error_message   text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    completed_at    timestamptz
);

CREATE INDEX IF NOT EXISTS import_jobs_landlord_idx ON import_jobs (landlord_id, created_at DESC);

CREATE TABLE IF NOT EXISTS import_drafts (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    job_id          bigint NOT NULL REFERENCES import_jobs (id) ON DELETE CASCADE,
    row_no          integer NOT NULL,
    payload_json    jsonb NOT NULL DEFAULT '{}'::jsonb,
    status          text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'created', 'error')),
    error_message   text,
    property_uuid   uuid,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS import_drafts_job_idx ON import_drafts (job_id, row_no);
