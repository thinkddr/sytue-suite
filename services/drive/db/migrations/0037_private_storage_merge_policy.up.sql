-- SPDX-License-Identifier: Apache-2.0
-- The platform merge preflight must see every tenant table. Frozen snapshots
-- remain immutable, including during a merge: their authenticated client scope
-- cannot be renamed by rewriting SQL. Such a merge aborts atomically until a
-- verified client migration can move the encrypted data to its new scope.
CREATE POLICY drive_private_storage_states_platform_merge ON drive_private_storage_states
    USING (current_setting('app.merge',true)='on')
    WITH CHECK (current_setting('app.merge',true)='on');
CREATE POLICY drive_private_storage_snapshot_platform_merge ON drive_private_storage_snapshot
    USING (current_setting('app.merge',true)='on')
    WITH CHECK (current_setting('app.merge',true)='on');
CREATE POLICY drive_private_storage_source_blobs_platform_merge ON drive_private_storage_source_blobs
    USING (current_setting('app.merge',true)='on')
    WITH CHECK (current_setting('app.merge',true)='on');

CREATE FUNCTION drive_private_storage_source_immutable() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,public AS $$
BEGIN
    IF TG_TABLE_NAME<>'drive_private_storage_states' THEN
        RAISE EXCEPTION USING ERRCODE='PST01',MESSAGE='drive_private_storage_source_immutable';
    END IF;
    IF OLD.mode<>'legacy' AND (TG_OP='DELETE'
       OR NEW.tenant IS DISTINCT FROM OLD.tenant OR NEW.owner_sub IS DISTINCT FROM OLD.owner_sub
       OR NEW.epoch IS DISTINCT FROM OLD.epoch OR NEW.mode='legacy') THEN
        RAISE EXCEPTION USING ERRCODE='PST01',MESSAGE='drive_private_storage_scope_immutable';
    END IF;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER drive_private_storage_state_scope_guard BEFORE UPDATE OR DELETE ON drive_private_storage_states
    FOR EACH ROW EXECUTE FUNCTION drive_private_storage_source_immutable();
CREATE TRIGGER drive_private_storage_snapshot_immutable BEFORE UPDATE OR DELETE ON drive_private_storage_snapshot
    FOR EACH ROW EXECUTE FUNCTION drive_private_storage_source_immutable();
CREATE TRIGGER drive_private_storage_source_immutable BEFORE UPDATE OR DELETE ON drive_private_storage_source_blobs
    FOR EACH ROW EXECUTE FUNCTION drive_private_storage_source_immutable();

CREATE OR REPLACE FUNCTION drive_private_storage_legacy_guard(t text,o text) RETURNS void LANGUAGE plpgsql
SET search_path=pg_catalog,public AS $$
DECLARE state_mode text;
BEGIN
    -- The former tenant-wide root and shared/team/channel data need their own
    -- collaborative migration. They must not be assigned to an arbitrary user.
    IF o IS NULL OR o='' THEN RETURN; END IF;
    SELECT mode INTO state_mode FROM public.drive_private_storage_states
      WHERE tenant=t AND owner_sub=o FOR SHARE;
    IF NOT FOUND THEN
        -- Merge rewrites the state table before files/folders. Recreating an
        -- absent old tenant here would leave rows behind after a successful
        -- merge. Existing states are still locked and frozen owners rejected.
        IF current_setting('app.merge',true)='on' THEN RETURN; END IF;
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

