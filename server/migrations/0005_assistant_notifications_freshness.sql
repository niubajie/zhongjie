-- 0005_assistant_notifications_freshness.sql
-- Notification queue, freshness checks, assistant sessions and narration cache.

CREATE TABLE IF NOT EXISTS notification_outbox (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_type      text NOT NULL,
    recipient_id    bigint NOT NULL REFERENCES users (id),
    payload_enc     bytea NOT NULL,
    status          text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'sent', 'failed', 'not_configured')),
    attempts        integer NOT NULL DEFAULT 0,
    next_retry_at   timestamptz NOT NULL DEFAULT now(),
    last_error      text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS notification_outbox_due_idx
    ON notification_outbox (status, next_retry_at);

CREATE TABLE IF NOT EXISTS freshness_checks (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    property_id     bigint NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    sent_at         timestamptz,
    responded_at    timestamptz,
    response        text CHECK (response IN ('active', 'rented', 'offline')),
    status          text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'responded', 'expired')),
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS freshness_checks_property_idx ON freshness_checks (property_id, created_at DESC);

CREATE TABLE IF NOT EXISTS assistant_sessions (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    uuid            uuid NOT NULL DEFAULT uuidv7(),
    user_id         bigint REFERENCES users (id),
    state           text NOT NULL DEFAULT 'collecting',
    slots_json      jsonb NOT NULL DEFAULT '{}'::jsonb,
    last_intent     text,
    expires_at      timestamptz NOT NULL DEFAULT now() + interval '2 hours',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS assistant_sessions_uuid_uq ON assistant_sessions (uuid);
CREATE INDEX IF NOT EXISTS assistant_sessions_user_idx ON assistant_sessions (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS recommendation_runs (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session_uuid    uuid NOT NULL,
    filters_json    jsonb NOT NULL DEFAULT '{}'::jsonb,
    result_json     jsonb NOT NULL DEFAULT '[]'::jsonb,
    model_version   text NOT NULL DEFAULT 'rule-v1',
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS recommendation_runs_session_idx ON recommendation_runs (session_uuid, created_at DESC);

CREATE TABLE IF NOT EXISTS narration_scripts (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    property_id     bigint NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    version         integer NOT NULL DEFAULT 1,
    segments_json   jsonb NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (property_id, version)
);

CREATE TABLE IF NOT EXISTS property_media (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    property_id     bigint NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    media_type      text NOT NULL CHECK (media_type IN ('video', 'floorplan', '3d_tour')),
    object_key      text NOT NULL,
    cover_key       text,
    duration_ms     integer,
    sort            integer NOT NULL DEFAULT 0,
    moderation_status text NOT NULL DEFAULT 'pending'
                    CHECK (moderation_status IN ('pending', 'approved', 'rejected')),
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS property_media_property_idx ON property_media (property_id, media_type, sort);
