-- SPDX-License-Identifier: Apache-2.0
-- Only opaque room identifiers and short-lived encrypted login grants live here.
CREATE TABLE matrix_draft_rooms (
    tenant text NOT NULL,
    subject text NOT NULL,
    room_id text PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX matrix_draft_rooms_actor ON matrix_draft_rooms(tenant,subject,created_at DESC);
ALTER TABLE matrix_draft_rooms ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_draft_rooms FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_draft_rooms_tenant ON matrix_draft_rooms
    USING(tenant=current_setting('app.tenant',true)) WITH CHECK(tenant=current_setting('app.tenant',true));
CREATE POLICY matrix_draft_rooms_maintenance ON matrix_draft_rooms
    USING(current_setting('app.maintenance',true)='on') WITH CHECK(current_setting('app.maintenance',true)='on');
CREATE POLICY matrix_draft_rooms_platform_merge ON matrix_draft_rooms
    USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');

CREATE TABLE matrix_native_consents (
    tenant text NOT NULL,
    state_hash text PRIMARY KEY,
    subject text NOT NULL,
    challenge text NOT NULL CHECK(challenge~'^[A-Za-z0-9_-]{43}$'),
    login_secret text NOT NULL DEFAULT '',
    ticket_hash text NOT NULL DEFAULT '',
    expires_at timestamptz NOT NULL,
    ticket_expires_at timestamptz
);
CREATE INDEX matrix_native_consents_expiry ON matrix_native_consents(expires_at);
ALTER TABLE matrix_native_consents ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_native_consents FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_native_consents_tenant ON matrix_native_consents
    USING(tenant=current_setting('app.tenant',true)) WITH CHECK(tenant=current_setting('app.tenant',true));
CREATE POLICY matrix_native_consents_platform_merge ON matrix_native_consents
    USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');

-- These actor+tenant-bound, expiring grants must not survive a tenant move.
-- Keep encrypted draft references (room_id PK) from both organizations.
CREATE OR REPLACE FUNCTION platform_merge_prepare(source_tenant text,target_tenant text) RETURNS text
LANGUAGE plpgsql AS $$
DECLARE merged bigint; discarded bigint;
BEGIN
    IF current_setting('app.merge',true) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION 'Organization merge context required';
    END IF;
    IF source_tenant=target_tenant THEN RETURN NULL; END IF;
    IF EXISTS(SELECT 1 FROM matrix_tenant_access s JOIN matrix_tenant_access d USING(subject)
        WHERE s.tenant=source_tenant AND d.tenant=target_tenant AND s.version=d.version AND s.status<>d.status) THEN
        RAISE EXCEPTION 'Conflicting Matrix lifecycle commands share a version';
    END IF;
    UPDATE matrix_tenant_access d SET status=s.status,version=s.version,applied=s.applied
        FROM matrix_tenant_access s WHERE s.tenant=source_tenant AND d.tenant=target_tenant
        AND s.subject=d.subject AND s.version>d.version;
    DELETE FROM matrix_tenant_access s USING matrix_tenant_access d
        WHERE s.tenant=source_tenant AND d.tenant=target_tenant AND s.subject=d.subject;
    GET DIAGNOSTICS merged=ROW_COUNT;
    DELETE FROM matrix_native_consents WHERE tenant=source_tenant;
    GET DIAGNOSTICS discarded=ROW_COUNT;
    IF merged>0 OR discarded>0 THEN
        RETURN 'Matrix lifecycle versions retained; expired tenant-bound native sign-ins discarded; encrypted draft rooms preserved';
    END IF;
    RETURN NULL;
END $$;
