-- SPDX-License-Identifier: Apache-2.0
-- Old Photos policies omitted the common merge flag. Only legacy rows can
-- move: retained captures declare an explicit non-writing Scope contract and
-- existing domain guards reject frozen owner changes independently.
CREATE POLICY assets_platform_merge ON assets USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
CREATE POLICY albums_platform_merge ON albums USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
CREATE POLICY edits_platform_merge ON edits USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
CREATE POLICY album_assets_platform_merge ON album_assets USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
ALTER POLICY org_settings_platform_merge ON org_settings USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
-- sytue:scope-bound-merge-v1
SELECT sytue_scope_merge_register('photos_private_storage_snapshot'::regclass);
SELECT sytue_scope_merge_register('photos_private_storage_preflights'::regclass);
SELECT sytue_scope_merge_register('photos_private_storage_preflight_inventory'::regclass);
-- Capture identity and its first accepted routing are retained even for a
-- maintenance connection; only the initial plan CAS may add routing.
CREATE FUNCTION photos_private_preflight_scope_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR (to_jsonb(NEW)-'plan_room_id'-'plan_event_id') IS DISTINCT FROM (to_jsonb(OLD)-'plan_room_id'-'plan_event_id')
 OR (OLD.plan_event_id IS NOT NULL AND (NEW.plan_room_id,NEW.plan_event_id) IS DISTINCT FROM (OLD.plan_room_id,OLD.plan_event_id)) THEN
  RAISE EXCEPTION 'Photos preflight scope and accepted routing are retained' USING ERRCODE='PST01';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER photos_private_preflight_scope_immutable BEFORE UPDATE OR DELETE ON photos_private_storage_preflights
 FOR EACH ROW EXECUTE FUNCTION photos_private_preflight_scope_immutable();
-- sytue:scope-admission-v1
SELECT sytue_scope_admission_register_existing();
