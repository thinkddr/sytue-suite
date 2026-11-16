-- SPDX-License-Identifier: Apache-2.0
DROP TRIGGER drive_private_storage_source_immutable ON drive_private_storage_source_blobs;
DROP TRIGGER drive_private_storage_snapshot_immutable ON drive_private_storage_snapshot;
DROP TRIGGER drive_private_storage_state_scope_guard ON drive_private_storage_states;
DROP FUNCTION drive_private_storage_source_immutable();
DROP POLICY drive_private_storage_source_blobs_platform_merge ON drive_private_storage_source_blobs;
DROP POLICY drive_private_storage_snapshot_platform_merge ON drive_private_storage_snapshot;
DROP POLICY drive_private_storage_states_platform_merge ON drive_private_storage_states;

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

