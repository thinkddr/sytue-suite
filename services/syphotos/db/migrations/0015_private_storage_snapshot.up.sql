-- SPDX-License-Identifier: Apache-2.0
-- Routing and immutable retained source; no key or new private body lives here.
-- sytue:private-storage-fence-v1
CREATE TABLE photos_private_storage_states (
 tenant text NOT NULL,
 owner_sub text NOT NULL CHECK(owner_sub<>''),
 mode text NOT NULL DEFAULT 'legacy' CHECK(mode IN('legacy','building','frozen','private')),
 epoch uuid,
 started_at timestamptz,
 source_rows bigint NOT NULL DEFAULT 0 CHECK(source_rows>=0),
 source_blobs bigint NOT NULL DEFAULT 0 CHECK(source_blobs>=0 AND source_blobs<=source_rows),
 PRIMARY KEY(tenant,owner_sub),
 CHECK((mode='legacy' AND epoch IS NULL AND started_at IS NULL) OR (mode<>'legacy' AND epoch IS NOT NULL AND started_at IS NOT NULL))
);
ALTER TABLE photos_private_storage_states ENABLE ROW LEVEL SECURITY;
ALTER TABLE photos_private_storage_states FORCE ROW LEVEL SECURITY;
CREATE POLICY photos_private_state_owner ON photos_private_storage_states
 USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true))
 WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY photos_private_state_maintenance ON photos_private_storage_states
 USING(current_setting('app.maintenance',true)='on') WITH CHECK(current_setting('app.maintenance',true)='on');
CREATE POLICY photos_private_state_merge ON photos_private_storage_states
 USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
CREATE TRIGGER photos_private_state_scope_guard BEFORE UPDATE OR DELETE ON photos_private_storage_states
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_state_scope_guard();
CREATE CONSTRAINT TRIGGER photos_private_capture_completed AFTER INSERT OR UPDATE ON photos_private_storage_states
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_build_completed();

CREATE TABLE photos_private_storage_snapshot (
 tenant text NOT NULL,owner_sub text NOT NULL,epoch uuid NOT NULL,
 kind text COLLATE "C" NOT NULL CHECK(kind IN('asset','album','edit','album_asset','album_person','media','variant')),
 id text COLLATE "C" NOT NULL,payload bytea,size_bytes bigint NOT NULL CHECK(size_bytes>=0 AND size_bytes<=140737488355328),
 sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),storage_key text,
 PRIMARY KEY(tenant,owner_sub,epoch,kind,id),
 FOREIGN KEY(tenant,owner_sub) REFERENCES photos_private_storage_states(tenant,owner_sub),
 CHECK((kind='variant' AND payload IS NULL AND storage_key IS NOT NULL AND storage_key<>'') OR
       (kind<>'variant' AND payload IS NOT NULL AND storage_key IS NULL AND octet_length(payload) BETWEEN 2 AND 33554432 AND size_bytes=octet_length(payload) AND sha256=encode(digest(payload,'sha256'),'hex')))
);
ALTER TABLE photos_private_storage_snapshot ENABLE ROW LEVEL SECURITY;
ALTER TABLE photos_private_storage_snapshot FORCE ROW LEVEL SECURITY;
CREATE POLICY photos_private_snapshot_read ON photos_private_storage_snapshot FOR SELECT
 USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY photos_private_snapshot_capture ON photos_private_storage_snapshot FOR INSERT
 WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true)
 AND EXISTS(SELECT 1 FROM photos_private_storage_states s WHERE s.tenant=photos_private_storage_snapshot.tenant AND s.owner_sub=photos_private_storage_snapshot.owner_sub AND s.epoch=photos_private_storage_snapshot.epoch AND s.mode='building'));
CREATE TRIGGER photos_private_snapshot_immutable BEFORE UPDATE OR DELETE ON photos_private_storage_snapshot
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_snapshot_immutable();
CREATE FUNCTION photos_private_snapshot_insert_guard() RETURNS trigger LANGUAGE plpgsql
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.photos_private_storage_states s WHERE s.tenant=NEW.tenant AND s.owner_sub=NEW.owner_sub AND s.epoch=NEW.epoch AND s.mode='building') THEN
  RAISE EXCEPTION USING ERRCODE='PST01',MESSAGE='private_storage_source_immutable';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER photos_private_snapshot_insert_guard BEFORE INSERT ON photos_private_storage_snapshot
 FOR EACH ROW EXECUTE FUNCTION photos_private_snapshot_insert_guard();

-- Preflight stores only a CAS of existing retained metadata/variant inventory
-- and the encrypted plan's opaque routing. No original pin or device key.
CREATE TABLE photos_private_storage_preflights (
 tenant text NOT NULL,owner_sub text NOT NULL,epoch uuid NOT NULL,ticket uuid NOT NULL,
 inventory_sha256 text NOT NULL CHECK(inventory_sha256 ~ '^[a-f0-9]{64}$'),
 backend_rows bigint NOT NULL CHECK(backend_rows>=0),backend_blobs bigint NOT NULL CHECK(backend_blobs>=0 AND backend_blobs<=backend_rows),
 original_blobs bigint NOT NULL CHECK(original_blobs>=0),
 plan_room_id text,plan_event_id text,
 PRIMARY KEY(tenant,owner_sub,epoch),FOREIGN KEY(tenant,owner_sub) REFERENCES photos_private_storage_states(tenant,owner_sub),
 CHECK((plan_room_id IS NULL AND plan_event_id IS NULL) OR (plan_room_id IS NOT NULL AND plan_event_id IS NOT NULL))
);
ALTER TABLE photos_private_storage_preflights ENABLE ROW LEVEL SECURITY;
ALTER TABLE photos_private_storage_preflights FORCE ROW LEVEL SECURITY;
CREATE POLICY photos_private_preflight_owner ON photos_private_storage_preflights
 USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true))
 WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE FUNCTION photos_private_preflight_guard() RETURNS trigger LANGUAGE plpgsql
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE value jsonb;
BEGIN
 FOR value IN SELECT v FROM(VALUES(CASE WHEN TG_OP<>'INSERT' THEN to_jsonb(OLD) END),(CASE WHEN TG_OP<>'DELETE' THEN to_jsonb(NEW) END))x(v) WHERE v IS NOT NULL LOOP
  PERFORM public.sytue_private_storage_guard('public.photos_private_storage_states'::regclass,value->>'tenant',value->>'owner_sub');
 END LOOP;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;RETURN NEW;
END;
$$;
CREATE TRIGGER photos_private_preflight_guard BEFORE INSERT OR UPDATE OR DELETE ON photos_private_storage_preflights
 FOR EACH ROW EXECUTE FUNCTION photos_private_preflight_guard();

-- OLD and NEW both participate, including moving a child, cover or incoming
-- grant. The common row guard takes the same shared owner lock as S3 writers.
CREATE POLICY photos_private_asset_routing ON assets FOR SELECT USING(current_setting('app.maintenance',true)='on');
CREATE POLICY photos_private_album_routing ON albums FOR SELECT USING(current_setting('app.maintenance',true)='on');
CREATE FUNCTION photos_private_row_guard() RETURNS trigger LANGUAGE plpgsql
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE value jsonb;candidate record;
BEGIN
 FOR value IN SELECT v FROM(VALUES(CASE WHEN TG_OP<>'INSERT' THEN to_jsonb(OLD) END),(CASE WHEN TG_OP<>'DELETE' THEN to_jsonb(NEW) END))x(v) WHERE v IS NOT NULL LOOP
  FOR candidate IN SELECT DISTINCT t,o FROM (
   SELECT value->>'tenant' t,value->>'owner_sub' o WHERE TG_TABLE_NAME IN('assets','albums','edits')
   UNION SELECT a.tenant,a.owner_sub FROM public.assets a WHERE a.tenant=value->>'tenant' AND a.id::text IN(value->>'asset_id',value->>'cover_asset_id')
   UNION SELECT a.tenant,a.owner_sub FROM public.albums a WHERE a.tenant=value->>'tenant' AND a.id::text=value->>'album_id'
   UNION SELECT value->>'tenant',value->>'sujeto' WHERE TG_TABLE_NAME='album_personas'
  ) owners WHERE coalesce(o,'')<>'' ORDER BY t,o LOOP
   PERFORM public.sytue_private_storage_guard('public.photos_private_storage_states'::regclass,candidate.t,candidate.o);
  END LOOP;
 END LOOP;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;RETURN NEW;
END;
$$;
CREATE TRIGGER photos_private_assets BEFORE INSERT OR UPDATE OR DELETE ON assets FOR EACH ROW EXECUTE FUNCTION photos_private_row_guard();
CREATE TRIGGER photos_private_albums BEFORE INSERT OR UPDATE OR DELETE ON albums FOR EACH ROW EXECUTE FUNCTION photos_private_row_guard();
CREATE TRIGGER photos_private_edits BEFORE INSERT OR UPDATE OR DELETE ON edits FOR EACH ROW EXECUTE FUNCTION photos_private_row_guard();
CREATE TRIGGER photos_private_members BEFORE INSERT OR UPDATE OR DELETE ON album_assets FOR EACH ROW EXECUTE FUNCTION photos_private_row_guard();
CREATE TRIGGER photos_private_people BEFORE INSERT OR UPDATE OR DELETE ON album_personas FOR EACH ROW EXECUTE FUNCTION photos_private_row_guard();
