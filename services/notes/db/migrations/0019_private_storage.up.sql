-- SPDX-License-Identifier: Apache-2.0
-- Retained legacy source only. Devices publish and verify its E2E replacement;
-- neither this migration nor a source read deletes the original application data.
CREATE TABLE notes_private_storage_states (
 tenant text NOT NULL,
 owner_sub text NOT NULL CHECK(owner_sub<>''),
 mode text NOT NULL DEFAULT 'legacy' CHECK(mode IN('legacy','building','frozen','private')),
 epoch uuid,
 started_at timestamptz,
 source_rows bigint NOT NULL DEFAULT 0 CHECK(source_rows>=0),
 source_blobs bigint NOT NULL DEFAULT 0 CHECK(source_blobs>=0 AND source_blobs<=source_rows),
 PRIMARY KEY(tenant,owner_sub),
 CHECK((mode='legacy' AND epoch IS NULL AND started_at IS NULL) OR
       (mode<>'legacy' AND epoch IS NOT NULL AND started_at IS NOT NULL))
);
ALTER TABLE notes_private_storage_states ENABLE ROW LEVEL SECURITY;
ALTER TABLE notes_private_storage_states FORCE ROW LEVEL SECURITY;
CREATE POLICY notes_private_state_owner ON notes_private_storage_states
 USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true))
 WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY notes_private_state_maintenance ON notes_private_storage_states
 USING(current_setting('app.maintenance',true)='on') WITH CHECK(current_setting('app.maintenance',true)='on');
CREATE POLICY notes_private_state_merge ON notes_private_storage_states
 USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');

CREATE TABLE notes_private_storage_snapshot (
 tenant text NOT NULL,
 owner_sub text NOT NULL,
 epoch uuid NOT NULL,
 kind text COLLATE "C" NOT NULL CHECK(kind IN('notebook','note','tag','tag_state','tag_mark','note_link','reminder','audit')),
 id text COLLATE "C" NOT NULL,
 payload bytea NOT NULL CHECK(octet_length(payload) BETWEEN 2 AND 33554432),
 size_bytes bigint GENERATED ALWAYS AS(octet_length(payload)::bigint) STORED,
 sha256 text GENERATED ALWAYS AS(encode(digest(payload,'sha256'),'hex')) STORED,
 PRIMARY KEY(tenant,owner_sub,epoch,kind,id),
 FOREIGN KEY(tenant,owner_sub) REFERENCES notes_private_storage_states(tenant,owner_sub)
);
ALTER TABLE notes_private_storage_snapshot ENABLE ROW LEVEL SECURITY;
ALTER TABLE notes_private_storage_snapshot FORCE ROW LEVEL SECURITY;
CREATE POLICY notes_private_snapshot_owner ON notes_private_storage_snapshot FOR SELECT
 USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY notes_private_snapshot_build ON notes_private_storage_snapshot FOR INSERT
 WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true)
  AND EXISTS(SELECT 1 FROM notes_private_storage_states s WHERE s.tenant=notes_private_storage_snapshot.tenant
   AND s.owner_sub=notes_private_storage_snapshot.owner_sub AND s.epoch=notes_private_storage_snapshot.epoch AND s.mode='building'));
CREATE POLICY notes_private_snapshot_maintenance ON notes_private_storage_snapshot FOR SELECT
 USING(current_setting('app.maintenance',true)='on');
CREATE POLICY notes_private_snapshot_merge ON notes_private_storage_snapshot
 USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');

-- sytue:private-storage-fence-v1
CREATE CONSTRAINT TRIGGER notes_private_build_completed
 AFTER INSERT OR UPDATE ON notes_private_storage_states DEFERRABLE INITIALLY DEFERRED
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_build_completed();

CREATE TRIGGER notes_private_state_immutable BEFORE UPDATE OR DELETE ON notes_private_storage_states
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_state_scope_guard();
CREATE TRIGGER notes_private_snapshot_immutable BEFORE UPDATE OR DELETE ON notes_private_storage_snapshot
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_snapshot_immutable();
ALTER TABLE notes_private_storage_states ENABLE ALWAYS TRIGGER notes_private_state_immutable;
ALTER TABLE notes_private_storage_snapshot ENABLE ALWAYS TRIGGER notes_private_snapshot_immutable;

-- Routing is Notes domain policy. Locking and the old-snapshot upsert guard live
-- once in the versioned pg helper above, shared with Mail and Drive.
CREATE FUNCTION notes_private_row_owners(tab text,row_value jsonb)
RETURNS TABLE(tenant text,owner_sub text) LANGUAGE sql STABLE
SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
 WITH route AS (
  SELECT row_value->>'tenant' AS t,
   CASE WHEN tab='notes' THEN row_value->>'id' ELSE row_value->>'note_id' END AS note_id,
   CASE WHEN tab='notebooks' THEN row_value->>'id' ELSE row_value->>'notebook_id' END AS book_id,
   CASE WHEN tab='tags' THEN row_value->>'id' ELSE row_value->>'tag_id' END AS tag_id
 ),owners AS (
  SELECT t,row_value->>'owner_sub' AS o FROM route
  UNION SELECT t,row_value->>'sujeto' FROM route WHERE tab IN('notebook_lectores','notebook_editores','notebook_peticiones_edicion','note_personas')
  UNION SELECT n.tenant,n.owner_sub FROM route r JOIN notes n ON n.tenant=r.t AND n.id::text=r.note_id
  UNION SELECT nb.tenant,nb.owner_sub FROM route r JOIN notes n ON n.tenant=r.t AND n.id::text=r.note_id
   JOIN notebooks nb ON nb.tenant=n.tenant AND nb.id=n.notebook_id
  UNION SELECT nb.tenant,nb.owner_sub FROM route r JOIN notebooks nb ON nb.tenant=r.t AND nb.id::text=r.book_id
  UNION SELECT nb.tenant,nb.owner_sub FROM route r JOIN notebooks nb ON nb.tenant=r.t AND nb.id::text=row_value->>'parent_id' WHERE tab='notebooks'
  UNION SELECT n.tenant,n.owner_sub FROM route r JOIN notes n ON n.tenant=r.t AND n.id::text=row_value->>'group_id' WHERE tab='notes'
  UNION SELECT t.tenant,t.owner_sub FROM route r JOIN tags t ON t.tenant=r.t AND t.id::text=r.tag_id
 ) SELECT DISTINCT owners.t,owners.o FROM owners WHERE owners.t IS NOT NULL AND owners.o IS NOT NULL AND owners.o<>''
$$;
CREATE FUNCTION notes_private_row_guard() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE old_value jsonb;new_value jsonb;own record;
BEGIN
 IF TG_OP<>'INSERT' THEN old_value:=to_jsonb(OLD); END IF;
 IF TG_OP<>'DELETE' THEN new_value:=to_jsonb(NEW); END IF;
 FOR own IN SELECT tenant,owner_sub FROM notes_private_row_owners(TG_TABLE_NAME,old_value)
  UNION SELECT tenant,owner_sub FROM notes_private_row_owners(TG_TABLE_NAME,new_value)
  ORDER BY tenant,owner_sub LOOP
  PERFORM sytue_private_storage_guard('public.notes_private_storage_states'::regclass,own.tenant,own.owner_sub);
 END LOOP;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER notes_private_notebooks BEFORE INSERT OR UPDATE OR DELETE ON notebooks FOR EACH ROW EXECUTE FUNCTION notes_private_row_guard();
CREATE TRIGGER notes_private_notes BEFORE INSERT OR UPDATE OR DELETE ON notes FOR EACH ROW EXECUTE FUNCTION notes_private_row_guard();
CREATE TRIGGER notes_private_tags BEFORE INSERT OR UPDATE OR DELETE ON tags FOR EACH ROW EXECUTE FUNCTION notes_private_row_guard();
CREATE TRIGGER notes_private_tag_states BEFORE INSERT OR UPDATE OR DELETE ON tag_states FOR EACH ROW EXECUTE FUNCTION notes_private_row_guard();
CREATE TRIGGER notes_private_marks BEFORE INSERT OR UPDATE OR DELETE ON note_tag_states FOR EACH ROW EXECUTE FUNCTION notes_private_row_guard();
CREATE TRIGGER notes_private_links BEFORE INSERT OR UPDATE OR DELETE ON note_links FOR EACH ROW EXECUTE FUNCTION notes_private_row_guard();
CREATE TRIGGER notes_private_reminders BEFORE INSERT OR UPDATE OR DELETE ON note_reminders FOR EACH ROW EXECUTE FUNCTION notes_private_row_guard();
CREATE TRIGGER notes_private_readers BEFORE INSERT OR UPDATE OR DELETE ON notebook_lectores FOR EACH ROW EXECUTE FUNCTION notes_private_row_guard();
CREATE TRIGGER notes_private_editors BEFORE INSERT OR UPDATE OR DELETE ON notebook_editores FOR EACH ROW EXECUTE FUNCTION notes_private_row_guard();
CREATE TRIGGER notes_private_requests BEFORE INSERT OR UPDATE OR DELETE ON notebook_peticiones_edicion FOR EACH ROW EXECUTE FUNCTION notes_private_row_guard();
CREATE TRIGGER notes_private_people BEFORE INSERT OR UPDATE OR DELETE ON note_personas FOR EACH ROW EXECUTE FUNCTION notes_private_row_guard();
ALTER TABLE notebooks ENABLE ALWAYS TRIGGER notes_private_notebooks;
ALTER TABLE notes ENABLE ALWAYS TRIGGER notes_private_notes;
ALTER TABLE tags ENABLE ALWAYS TRIGGER notes_private_tags;
ALTER TABLE tag_states ENABLE ALWAYS TRIGGER notes_private_tag_states;
ALTER TABLE note_tag_states ENABLE ALWAYS TRIGGER notes_private_marks;
ALTER TABLE note_links ENABLE ALWAYS TRIGGER notes_private_links;
ALTER TABLE note_reminders ENABLE ALWAYS TRIGGER notes_private_reminders;
ALTER TABLE notebook_lectores ENABLE ALWAYS TRIGGER notes_private_readers;
ALTER TABLE notebook_editores ENABLE ALWAYS TRIGGER notes_private_editors;
ALTER TABLE notebook_peticiones_edicion ENABLE ALWAYS TRIGGER notes_private_requests;
ALTER TABLE note_personas ENABLE ALWAYS TRIGGER notes_private_people;
