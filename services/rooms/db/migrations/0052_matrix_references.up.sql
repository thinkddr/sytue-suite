-- SPDX-License-Identifier: Apache-2.0
CREATE TABLE matrix_channel_rooms (
    tenant text NOT NULL,
    channel_id uuid NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    room_id text NOT NULL,
    PRIMARY KEY (tenant, channel_id),
    UNIQUE (room_id)
);
ALTER TABLE matrix_channel_rooms ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_channel_rooms FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_channel_rooms_tenant ON matrix_channel_rooms
    USING (tenant=current_setting('app.tenant',true))
    WITH CHECK (tenant=current_setting('app.tenant',true));
CREATE TABLE matrix_tenant_access (
    tenant text NOT NULL,
    subject text NOT NULL,
    status text NOT NULL,
    version bigint NOT NULL,
    applied boolean NOT NULL DEFAULT false,
    PRIMARY KEY(tenant,subject)
);
ALTER TABLE matrix_tenant_access ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_tenant_access FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_tenant_access_tenant ON matrix_tenant_access
    USING(tenant=current_setting('app.tenant',true)) WITH CHECK(tenant=current_setting('app.tenant',true));
ALTER TABLE messages ADD COLUMN matrix_room_id text NOT NULL DEFAULT '',
                     ADD COLUMN matrix_event_id text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX messages_matrix_event ON messages(tenant,matrix_room_id,matrix_event_id)
    WHERE matrix_event_id<>'';
ALTER TABLE messages DROP CONSTRAINT messages_body_not_blank;
ALTER TABLE messages ADD CONSTRAINT messages_body_not_blank CHECK (
    (matrix_room_id='' AND matrix_event_id='' AND (length(body)>0 OR type<>'user' OR attachments<>'[]'::jsonb))
 OR (matrix_room_id<>'' AND matrix_event_id<>'' AND body='' AND attachments='[]'::jsonb
     AND mentions='{}'::text[] AND NOT mentions_all AND card='{}'::jsonb)
);

CREATE POLICY matrix_channel_rooms_maintenance ON matrix_channel_rooms USING(current_setting('app.maintenance',true)='on');
CREATE TABLE matrix_identity_access (
    subject text PRIMARY KEY,
    status text NOT NULL DEFAULT 'active',
    version bigint NOT NULL DEFAULT 0,
    applied boolean NOT NULL DEFAULT true
);
ALTER TABLE matrix_identity_access ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_identity_access FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_identity_access_maintenance ON matrix_identity_access
    USING(current_setting('app.maintenance',true)='on') WITH CHECK(current_setting('app.maintenance',true)='on');

CREATE POLICY matrix_channel_rooms_platform_merge ON matrix_channel_rooms
    USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
CREATE POLICY matrix_tenant_access_platform_merge ON matrix_tenant_access
    USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
ALTER TABLE matrix_tenant_access ADD CONSTRAINT matrix_tenant_access_status CHECK(status IN ('active','removed')),
    ADD CONSTRAINT matrix_tenant_access_version CHECK(version>=0);
ALTER TABLE matrix_identity_access ADD CONSTRAINT matrix_identity_access_status CHECK(status IN ('active','suspended','deleted')),
    ADD CONSTRAINT matrix_identity_access_version CHECK(version>=0);

-- Access versions are durable tombstones, not personal content. Account deletion
-- must never purge them: delayed active commands must remain older than a denial.
-- The existing service-owned merge hook resolves an overlapping subject without
-- losing the highest ID sequence. All other tables still use the generic mover.
CREATE FUNCTION platform_merge_prepare(source_tenant text,target_tenant text) RETURNS text
LANGUAGE plpgsql AS $$
DECLARE merged bigint;
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
    IF merged>0 THEN RETURN 'Matrix lifecycle access merged by highest ID version'; END IF;
    RETURN NULL;
END $$;
