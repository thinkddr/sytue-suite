DROP POLICY IF EXISTS audit_events_tenant_isolation ON audit_events;
DROP TABLE IF EXISTS audit_events;
ALTER TABLE env_vars DROP COLUMN IF EXISTS key_version;
ALTER TABLE deployments DROP COLUMN IF EXISTS image_ref;
ALTER TABLE builds DROP COLUMN IF EXISTS kaniko_job_name;
