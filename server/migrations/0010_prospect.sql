-- 0010_prospect.sql
-- 自主获客（方案 3.12 / 3.13，M10）数据模型：信息源、潜在线索、跟进记录、拒收名单。
-- 合规：原文 AES 加密存 raw_text_enc / content_enc；联系方式只存 contact_hash（HMAC-SHA256）；
-- 软删除表查询必须带 deleted_at IS NULL。停机开关默认关闭（lead_sources.enabled=false）。

-- 信息源：采集来源配置，也是单源停机开关的载体。
CREATE TABLE IF NOT EXISTS lead_sources (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    uuid            uuid NOT NULL DEFAULT uuidv7(),
    name            text NOT NULL,
    kind            text NOT NULL
                    CHECK (kind IN ('self_owned', 'public_api', 'third_party_scrape')),
    base_url        text,
    robots_ok       boolean NOT NULL DEFAULT false,
    terms_ok        boolean NOT NULL DEFAULT false,
    rate_limit_ms   integer NOT NULL DEFAULT 3000 CHECK (rate_limit_ms >= 0),
    enabled         boolean NOT NULL DEFAULT false,
    review_note     text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS lead_sources_name_uq
    ON lead_sources (name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS lead_sources_enabled_idx ON lead_sources (enabled) WHERE deleted_at IS NULL;

-- 潜在线索：采集到的原始公开信息，去重后入库，默认 status=new 等人工复核。
CREATE TABLE IF NOT EXISTS prospect_leads (
    id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    uuid                uuid NOT NULL DEFAULT uuidv7(),
    source_id           bigint NOT NULL REFERENCES lead_sources (id),
    lead_type           text NOT NULL CHECK (lead_type IN ('tenant', 'landlord')),
    platform            text NOT NULL,
    external_post_id    text NOT NULL,
    raw_text_enc        bytea,
    contact_hash        bytea,
    contact_channel     text NOT NULL DEFAULT 'unknown'
                        CHECK (contact_channel IN ('phone', 'wechat', 'in_app', 'sms', 'unknown')),
    city_code           text,
    district_code       text,
    budget_cents        bigint,
    area_sqm            numeric(6, 2),
    layout              text,
    intent_score        integer,
    intent_status       text NOT NULL DEFAULT 'unknown'
                        CHECK (intent_status IN (
                            'seeking', 'rented', 'landlord_listing', 'agent',
                            'irrelevant', 'unknown'
                        )),
    status              text NOT NULL DEFAULT 'new'
                        CHECK (status IN (
                            'new', 'reviewed', 'contacted', 'following_up',
                            'converted', 'published', 'cold', 'lost',
                            'opt_out', 'rejected', 'ignored'
                        )),
    matched_user_id     bigint,
    matched_property_id uuid,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    deleted_at          timestamptz
);

CREATE INDEX IF NOT EXISTS prospect_leads_type_status_idx
    ON prospect_leads (lead_type, status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS prospect_leads_contact_hash_idx
    ON prospect_leads (contact_hash) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS prospect_leads_source_post_uq
    ON prospect_leads (source_id, external_post_id) WHERE deleted_at IS NULL;

-- 跟进记录：每次联系（首条 + 后续提醒）都落一条；首条必须人工审核。
CREATE TABLE IF NOT EXISTS follow_ups (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    lead_id     bigint NOT NULL REFERENCES prospect_leads (id) ON DELETE CASCADE,
    channel     text NOT NULL
                CHECK (channel IN ('in_app', 'sms', 'wechat', 'phone')),
    direction   text NOT NULL
                CHECK (direction IN ('outbound', 'inbound')),
    content_enc bytea,
    status      text NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending', 'sent', 'failed', 'received')),
    sent_at     timestamptz,
    audit_id    bigint,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS follow_ups_lead_idx ON follow_ups (lead_id, created_at DESC);

-- 拒收名单：命中后该 contact_hash 在所有源 / 渠道停止联系，全局生效。
CREATE TABLE IF NOT EXISTS opt_outs (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    contact_hash    bytea NOT NULL,
    lead_type       text NOT NULL CHECK (lead_type IN ('tenant', 'landlord')),
    reason          text,
    opted_out_at    timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS opt_outs_contact_type_uq
    ON opt_outs (contact_hash, lead_type);

DROP TRIGGER IF EXISTS lead_sources_set_updated_at ON lead_sources;
CREATE TRIGGER lead_sources_set_updated_at BEFORE UPDATE ON lead_sources
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS prospect_leads_set_updated_at ON prospect_leads;
CREATE TRIGGER prospect_leads_set_updated_at BEFORE UPDATE ON prospect_leads
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
