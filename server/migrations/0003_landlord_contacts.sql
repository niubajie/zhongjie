-- 0003_landlord_contacts.sql
-- Contact preferences and the first contact-request path. WeChat ID is
-- optional and manually supplied by the landlord; it is never inferred from
-- WeChat.

CREATE TABLE IF NOT EXISTS landlord_profiles (
    user_id         bigint PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    landlord_code   text UNIQUE,
    wechat_id_enc   bytea,
    wechat_id_hash  bytea,
    company_name_enc bytea,
    share_phone     boolean NOT NULL DEFAULT true,
    share_wechat    boolean NOT NULL DEFAULT false,
    auto_publish    boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS landlord_profiles_wechat_hash_idx
    ON landlord_profiles (wechat_id_hash)
    WHERE wechat_id_hash IS NOT NULL;

DROP TRIGGER IF EXISTS landlord_profiles_set_updated_at ON landlord_profiles;
CREATE TRIGGER landlord_profiles_set_updated_at BEFORE UPDATE ON landlord_profiles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO landlord_profiles (user_id, share_phone)
SELECT id, true FROM users WHERE role = 'landlord'
ON CONFLICT (user_id) DO NOTHING;
