-- 0004_admin_users.sql
-- Management-site identities. Admin tokens use a separate issuer/secret from
-- tenant tokens; TOTP columns are reserved for the M2.1 hardening step.

CREATE TABLE IF NOT EXISTS admin_users (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    uuid            uuid NOT NULL DEFAULT uuidv7(),
    username        text NOT NULL,
    password_hash   text NOT NULL,
    role            text NOT NULL
                    CHECK (role IN ('super_admin', 'operations', 'reviewer', 'support', 'auditor')),
    status          text NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'disabled')),
    totp_secret_enc bytea,
    totp_enabled    boolean NOT NULL DEFAULT false,
    last_login_at   timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS admin_users_username_uq
    ON admin_users (username) WHERE deleted_at IS NULL;

DROP TRIGGER IF EXISTS admin_users_set_updated_at ON admin_users;
CREATE TRIGGER admin_users_set_updated_at BEFORE UPDATE ON admin_users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
