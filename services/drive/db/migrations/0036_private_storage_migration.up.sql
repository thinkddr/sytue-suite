-- SPDX-License-Identifier: Apache-2.0
-- Private migration of personal Drive data. No original is removed here.
-- Rows are frozen by owner before the metadata snapshot is captured. A shared
-- row lock in EVERY legacy writer closes the race with Begin's exclusive lock,
-- including transactions whose snapshot predates the migration.
CREATE TABLE drive_private_storage_states (
    tenant text NOT NULL,
    owner_sub text NOT NULL CHECK (owner_sub <> ''),
    mode text NOT NULL DEFAULT 'legacy' CHECK (mode IN ('legacy','building','frozen','private')),
    epoch uuid,
    started_at timestamptz,
    source_rows bigint NOT NULL DEFAULT 0 CHECK (source_rows>=0),
    source_blobs bigint NOT NULL DEFAULT 0 CHECK (source_blobs>=0 AND source_blobs<=source_rows),
    PRIMARY KEY (tenant,owner_sub),
    CHECK ((mode='legacy' AND epoch IS NULL AND started_at IS NULL)
        OR (mode<>'legacy' AND epoch IS NOT NULL AND started_at IS NOT NULL))
);
CREATE INDEX drive_private_storage_frozen ON drive_private_storage_states(tenant,owner_sub) WHERE mode<>'legacy';
ALTER TABLE drive_private_storage_states ENABLE ROW LEVEL SECURITY;
ALTER TABLE drive_private_storage_states FORCE ROW LEVEL SECURITY;
CREATE POLICY drive_private_storage_owner ON drive_private_storage_states
    USING (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true))
    WITH CHECK (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY drive_private_storage_maintenance ON drive_private_storage_states
    USING (current_setting('app.maintenance',true)='on')
    WITH CHECK (current_setting('app.maintenance',true)='on');

-- This is retained LEGACY data during migration, not an E2E store. It also
-- preserves personal relationships whose foreign resource can legitimately be
-- removed by another owner while migration is running. Clients encrypt it and
-- verify the replacement before a later, separately implemented retirement.
CREATE TABLE drive_private_storage_snapshot (
    tenant text NOT NULL,
    owner_sub text NOT NULL,
    epoch uuid NOT NULL,
    kind text COLLATE "C" NOT NULL CHECK (kind IN ('folder','file','version','share','favorite','request','provenance')),
    id text COLLATE "C" NOT NULL,
    payload bytea NOT NULL CHECK (octet_length(payload) BETWEEN 2 AND 65536),
    PRIMARY KEY (tenant,owner_sub,epoch,kind,id),
    FOREIGN KEY (tenant,owner_sub) REFERENCES drive_private_storage_states(tenant,owner_sub)
);
ALTER TABLE drive_private_storage_snapshot ENABLE ROW LEVEL SECURITY;
ALTER TABLE drive_private_storage_snapshot FORCE ROW LEVEL SECURITY;
CREATE POLICY drive_private_snapshot_read ON drive_private_storage_snapshot FOR SELECT
    USING (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY drive_private_snapshot_build ON drive_private_storage_snapshot FOR INSERT
    WITH CHECK (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true)
      AND EXISTS(SELECT 1 FROM drive_private_storage_states s
                 WHERE s.tenant=drive_private_storage_snapshot.tenant AND s.owner_sub=drive_private_storage_snapshot.owner_sub
                   AND s.epoch=drive_private_storage_snapshot.epoch AND s.mode='building'));
-- No UPDATE or DELETE policy: the snapshot cannot be edited or retired through
-- ordinary metadata calls, maintenance, or an unverified acknowledgement.
CREATE POLICY drive_private_snapshot_maintenance ON drive_private_storage_snapshot FOR SELECT
    USING (current_setting('app.maintenance',true)='on');

CREATE TABLE drive_private_storage_source_blobs (
    tenant text NOT NULL,
    owner_sub text NOT NULL,
    epoch uuid NOT NULL,
    kind text COLLATE "C" NOT NULL CHECK (kind IN ('file','version')),
    id text COLLATE "C" NOT NULL,
    storage_key text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    checksum text NOT NULL,
    PRIMARY KEY (tenant,owner_sub,epoch,kind,id),
    FOREIGN KEY (tenant,owner_sub,epoch,kind,id)
        REFERENCES drive_private_storage_snapshot(tenant,owner_sub,epoch,kind,id)
);
ALTER TABLE drive_private_storage_source_blobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE drive_private_storage_source_blobs FORCE ROW LEVEL SECURITY;
CREATE POLICY drive_private_source_read ON drive_private_storage_source_blobs FOR SELECT
    USING (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY drive_private_source_build ON drive_private_storage_source_blobs FOR INSERT
    WITH CHECK (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true)
      AND EXISTS(SELECT 1 FROM drive_private_storage_states s
                 WHERE s.tenant=drive_private_storage_source_blobs.tenant AND s.owner_sub=drive_private_storage_source_blobs.owner_sub
                   AND s.epoch=drive_private_storage_source_blobs.epoch AND s.mode='building'));
CREATE POLICY drive_private_source_maintenance ON drive_private_storage_source_blobs FOR SELECT
    USING (current_setting('app.maintenance',true)='on');
CREATE INDEX drive_private_source_storage_key ON drive_private_storage_source_blobs(storage_key);
CREATE INDEX drive_private_folders_source ON folders(tenant,owner_sub,id)
    WHERE team_drive_id IS NULL AND channel_id IS NULL;
CREATE INDEX drive_private_files_source ON files(tenant,owner_sub,id)
    WHERE team_drive_id IS NULL AND channel_id IS NULL;
CREATE INDEX drive_private_requests_source ON drive_peticiones_edicion(tenant,sujeto,recurso);

CREATE FUNCTION drive_private_storage_build_completed() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,public AS $$
BEGIN
    IF EXISTS(SELECT 1 FROM public.drive_private_storage_states s
              WHERE s.tenant=NEW.tenant AND s.owner_sub=NEW.owner_sub AND s.mode='building') THEN
        RAISE EXCEPTION USING ERRCODE='PST02',MESSAGE='drive_private_storage_snapshot_incomplete';
    END IF;
    RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER drive_private_storage_build_completed
    AFTER INSERT OR UPDATE ON drive_private_storage_states DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION drive_private_storage_build_completed();

CREATE FUNCTION drive_private_storage_legacy_guard(t text,o text) RETURNS void LANGUAGE plpgsql
SET search_path=pg_catalog,public AS $$
DECLARE state_mode text;
BEGIN
    -- The former tenant-wide root and shared/team/channel data need their own
    -- collaborative migration. They must not be assigned to an arbitrary user.
    IF o IS NULL OR o='' THEN RETURN; END IF;
    SELECT mode INTO state_mode FROM public.drive_private_storage_states
      WHERE tenant=t AND owner_sub=o FOR SHARE;
    IF NOT FOUND THEN
        -- ON CONFLICT also handles a state created after an old REPEATABLE READ
        -- snapshot: PostgreSQL aborts rather than letting that writer miss it.
        INSERT INTO public.drive_private_storage_states(tenant,owner_sub) VALUES(t,o)
        ON CONFLICT(tenant,owner_sub) DO UPDATE SET owner_sub=EXCLUDED.owner_sub
        RETURNING mode INTO state_mode;
    END IF;
    IF state_mode<>'legacy' THEN
        RAISE EXCEPTION USING ERRCODE='PST01',MESSAGE='drive_private_storage_legacy_write_blocked';
    END IF;
END;
$$;

-- The function-local maintenance context resolves ONLY routing ownership, then
-- restores the caller's RLS context automatically. It returns no private data.
-- SECURITY INVOKER: it grants no database privilege or BYPASSRLS role.
CREATE FUNCTION drive_private_storage_resource_guard(t text,resource uuid) RETURNS void LANGUAGE plpgsql
SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE target record;
BEGIN
    SELECT tenant,owner_sub,team_drive_id,channel_id INTO target FROM public.files WHERE tenant=t AND id=resource;
    IF NOT FOUND THEN
        SELECT tenant,owner_sub,team_drive_id,channel_id INTO target FROM public.folders WHERE tenant=t AND id=resource;
    END IF;
    IF FOUND AND target.team_drive_id IS NULL AND target.channel_id IS NULL THEN
        PERFORM public.drive_private_storage_legacy_guard(target.tenant,target.owner_sub);
    END IF;
END;
$$;
CREATE FUNCTION drive_private_storage_row_guard() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE row_value jsonb; which_row integer; first_row integer;
BEGIN
    first_row := CASE WHEN TG_OP='UPDATE' THEN 0 ELSE 1 END;
    FOR which_row IN first_row..1 LOOP
        row_value := CASE WHEN which_row=0 OR TG_OP='DELETE' THEN to_jsonb(OLD) ELSE to_jsonb(NEW) END;
        CASE TG_TABLE_NAME
        WHEN 'files','folders' THEN
            IF row_value->>'team_drive_id' IS NULL AND row_value->>'channel_id' IS NULL THEN
                PERFORM public.drive_private_storage_legacy_guard(row_value->>'tenant',row_value->>'owner_sub');
            END IF;
        WHEN 'favorites' THEN
            -- A foreign parent's legitimate cascade must not be held hostage by
            -- a personal favorite. Begin already preserved that relationship.
            IF NOT (TG_OP='DELETE' AND pg_trigger_depth()>1) THEN
                PERFORM public.drive_private_storage_legacy_guard(row_value->>'tenant',row_value->>'subject');
            END IF;
        WHEN 'drive_peticiones_edicion' THEN
            IF TG_OP<>'DELETE' THEN
                PERFORM public.drive_private_storage_legacy_guard(row_value->>'tenant',row_value->>'sujeto');
            END IF;
            PERFORM public.drive_private_storage_resource_guard(row_value->>'tenant',(row_value->>'recurso')::uuid);
        ELSE
            IF row_value->>'file_id' IS NOT NULL THEN
                PERFORM public.drive_private_storage_resource_guard(row_value->>'tenant',(row_value->>'file_id')::uuid);
            END IF;
            IF row_value->>'folder_id' IS NOT NULL THEN
                PERFORM public.drive_private_storage_resource_guard(row_value->>'tenant',(row_value->>'folder_id')::uuid);
            END IF;
        END CASE;
    END LOOP;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER drive_private_files_guard BEFORE INSERT OR UPDATE OR DELETE ON files FOR EACH ROW EXECUTE FUNCTION drive_private_storage_row_guard();
CREATE TRIGGER drive_private_folders_guard BEFORE INSERT OR UPDATE OR DELETE ON folders FOR EACH ROW EXECUTE FUNCTION drive_private_storage_row_guard();
CREATE TRIGGER drive_private_versions_guard BEFORE INSERT OR UPDATE OR DELETE ON file_versions FOR EACH ROW EXECUTE FUNCTION drive_private_storage_row_guard();
CREATE TRIGGER drive_private_shares_guard BEFORE INSERT OR UPDATE OR DELETE ON shares FOR EACH ROW EXECUTE FUNCTION drive_private_storage_row_guard();
CREATE TRIGGER drive_private_favorites_guard BEFORE INSERT OR UPDATE OR DELETE ON favorites FOR EACH ROW EXECUTE FUNCTION drive_private_storage_row_guard();
CREATE TRIGGER drive_private_requests_guard BEFORE INSERT OR UPDATE OR DELETE ON drive_peticiones_edicion FOR EACH ROW EXECUTE FUNCTION drive_private_storage_row_guard();
CREATE TRIGGER drive_private_provenance_guard BEFORE INSERT OR UPDATE OR DELETE ON drive_file_provenance FOR EACH ROW EXECUTE FUNCTION drive_private_storage_row_guard();
