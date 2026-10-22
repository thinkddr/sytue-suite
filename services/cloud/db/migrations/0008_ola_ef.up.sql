-- Ola E/F: cancelación de builds, imagen BYO, auditoría, rotación de claves.

ALTER TABLE builds ADD COLUMN IF NOT EXISTS kaniko_job_name text NOT NULL DEFAULT '';

ALTER TABLE deployments ADD COLUMN IF NOT EXISTS image_ref text NOT NULL DEFAULT '';

ALTER TABLE env_vars ADD COLUMN IF NOT EXISTS key_version smallint NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS audit_events (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    actor_sub   text        NOT NULL DEFAULT '',
    action      text        NOT NULL,
    resource    text        NOT NULL DEFAULT '',
    detail_json jsonb       NOT NULL DEFAULT '{}'::jsonb,
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS audit_events_tenant_time_idx
    ON audit_events (tenant, occurred_at DESC);

ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_events FORCE ROW LEVEL SECURITY;

CREATE POLICY audit_events_tenant_isolation ON audit_events
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
