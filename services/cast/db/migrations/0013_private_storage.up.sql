-- SPDX-License-Identifier: Apache-2.0
-- Retained historical source only. No new private content or key lives here.
-- sytue:private-storage-fence-v1
CREATE TABLE cast_private_storage_states (
 tenant text NOT NULL,owner_sub text NOT NULL CHECK(owner_sub<>''),
 mode text NOT NULL DEFAULT 'legacy' CHECK(mode IN('legacy','building','frozen','private')),
 epoch uuid,started_at timestamptz,
 source_rows bigint NOT NULL DEFAULT 0 CHECK(source_rows>=0),
 source_blobs bigint NOT NULL DEFAULT 0 CHECK(source_blobs>=0 AND source_blobs<=source_rows),
 PRIMARY KEY(tenant,owner_sub),
 CHECK((mode='legacy' AND epoch IS NULL AND started_at IS NULL) OR(mode<>'legacy' AND epoch IS NOT NULL AND started_at IS NOT NULL))
);
ALTER TABLE cast_private_storage_states ENABLE ROW LEVEL SECURITY;
ALTER TABLE cast_private_storage_states FORCE ROW LEVEL SECURITY;
CREATE POLICY cast_private_state_owner ON cast_private_storage_states
 USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true))
 WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY cast_private_state_maintenance ON cast_private_storage_states
 USING(current_setting('app.maintenance',true)='on') WITH CHECK(current_setting('app.maintenance',true)='on');
CREATE POLICY cast_private_state_merge ON cast_private_storage_states
 USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
CREATE TRIGGER cast_private_state_scope_guard BEFORE UPDATE OR DELETE ON cast_private_storage_states
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_state_scope_guard();
CREATE CONSTRAINT TRIGGER cast_private_capture_completed AFTER INSERT OR UPDATE ON cast_private_storage_states
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_build_completed();

CREATE TABLE cast_private_storage_snapshot (
 tenant text NOT NULL,owner_sub text NOT NULL,epoch uuid NOT NULL,
 kind text COLLATE "C" NOT NULL CHECK(kind IN('video','person','job','comment','chat','stream_key','viewer','presentation','slide_mark','report','person_block','external_access','external_comment','external_report','media','media_bytes')),
 id text COLLATE "C" NOT NULL,payload bytea,size_bytes bigint NOT NULL CHECK(size_bytes>=0 AND size_bytes<=140737488355328),
 sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),storage_key text,
 PRIMARY KEY(tenant,owner_sub,epoch,kind,id),
 FOREIGN KEY(tenant,owner_sub) REFERENCES cast_private_storage_states(tenant,owner_sub),
 CHECK((kind='media_bytes' AND payload IS NULL AND storage_key IS NOT NULL AND storage_key<>'') OR
  (kind<>'media_bytes' AND payload IS NOT NULL AND storage_key IS NULL AND octet_length(payload) BETWEEN 2 AND 33554432 AND size_bytes=octet_length(payload) AND sha256=encode(digest(payload,'sha256'),'hex')))
);
ALTER TABLE cast_private_storage_snapshot ENABLE ROW LEVEL SECURITY;
ALTER TABLE cast_private_storage_snapshot FORCE ROW LEVEL SECURITY;
CREATE POLICY cast_private_snapshot_read ON cast_private_storage_snapshot FOR SELECT
 USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY cast_private_snapshot_capture ON cast_private_storage_snapshot FOR INSERT
 WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true)
 AND EXISTS(SELECT 1 FROM cast_private_storage_states s WHERE s.tenant=cast_private_storage_snapshot.tenant AND s.owner_sub=cast_private_storage_snapshot.owner_sub AND s.epoch=cast_private_storage_snapshot.epoch AND s.mode='building'));
CREATE TRIGGER cast_private_snapshot_immutable BEFORE UPDATE OR DELETE ON cast_private_storage_snapshot
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_snapshot_immutable();
CREATE FUNCTION cast_private_snapshot_insert_guard() RETURNS trigger LANGUAGE plpgsql
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.cast_private_storage_states s WHERE s.tenant=NEW.tenant AND s.owner_sub=NEW.owner_sub AND s.epoch=NEW.epoch AND s.mode='building') THEN
  RAISE EXCEPTION USING ERRCODE='PST01',MESSAGE='private_storage_source_immutable';
 END IF;RETURN NEW;
END $$;
CREATE TRIGGER cast_private_snapshot_insert_guard BEFORE INSERT ON cast_private_storage_snapshot
 FOR EACH ROW EXECUTE FUNCTION cast_private_snapshot_insert_guard();
CREATE FUNCTION cast_private_retained_truncate() RETURNS trigger LANGUAGE plpgsql
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM public.cast_private_storage_states WHERE mode<>'legacy') THEN
  RAISE EXCEPTION 'Cast retained source cannot be truncated' USING ERRCODE='PST01';
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER cast_private_snapshot_truncate BEFORE TRUNCATE ON cast_private_storage_snapshot
 FOR EACH STATEMENT EXECUTE FUNCTION cast_private_retained_truncate();
CREATE TRIGGER cast_private_state_truncate BEFORE TRUNCATE ON cast_private_storage_states
 FOR EACH STATEMENT EXECUTE FUNCTION cast_private_retained_truncate();

-- Never infer private from org or people-with-zero-recipients. This additional
-- value is explicit; legacy public create/update validators still reject it.
ALTER TABLE videos DROP CONSTRAINT videos_visibility_check;
ALTER TABLE videos ADD CONSTRAINT videos_visibility_check CHECK(visibility IN('public','unlisted','org','people','private'));

-- Cast has global personal blocks and cross-tenant contributions. This lock
-- closes the missing-state insertion race for those relations; it supplements
-- (does not replace) the common owner ROW locks on all SQL and object effects.
CREATE FUNCTION cast_private_subject_lock(o text) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
 IF o IS NOT NULL AND o<>'' THEN
  PERFORM pg_advisory_xact_lock_shared(hashtextextended('cast-private-subject:'||o,0));
 END IF;
END $$;
CREATE FUNCTION cast_private_subject_freeze(o text) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
 IF o IS NULL OR o='' THEN RAISE EXCEPTION 'Cast private subject missing' USING ERRCODE='22023'; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended('cast-private-subject:'||o,0));
END $$;
CREATE FUNCTION cast_private_subject_guard(o text) RETURNS void LANGUAGE plpgsql
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE candidate record;
BEGIN
 IF coalesce(o,'')='' THEN RETURN; END IF;
 IF current_setting('transaction_isolation')<>'read committed' THEN
  RAISE EXCEPTION 'Cast global contribution requires fresh snapshot' USING ERRCODE='40001';
 END IF;
 PERFORM public.cast_private_subject_lock(o);
 FOR candidate IN SELECT tenant,owner_sub FROM public.cast_private_storage_states WHERE owner_sub=o ORDER BY tenant LOOP
  PERFORM public.sytue_private_storage_guard('public.cast_private_storage_states'::regclass,candidate.tenant,candidate.owner_sub);
 END LOOP;
END $$;
CREATE FUNCTION cast_private_row_guard() RETURNS trigger LANGUAGE plpgsql
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE value jsonb; t text; o text; actor text;
BEGIN
 FOR value IN SELECT v FROM(VALUES(CASE WHEN TG_OP<>'INSERT' THEN to_jsonb(OLD) END),(CASE WHEN TG_OP<>'DELETE' THEN to_jsonb(NEW) END))x(v) WHERE v IS NOT NULL LOOP
  t:=value->>'tenant';o:=NULL;
  IF TG_TABLE_NAME='person_blocks' THEN
   FOR actor IN SELECT DISTINCT v FROM(VALUES(value->>'blocker_sub'),(value->>'blocked_sub'))actors(v) ORDER BY v LOOP
    PERFORM public.cast_private_subject_guard(actor);
   END LOOP;
  ELSE
   IF TG_TABLE_NAME IN('videos','stream_keys') THEN o:=value->>'owner_sub';
   ELSE
    SELECT owner_sub INTO o FROM public.videos WHERE tenant=t AND id=(value->>'video_id')::uuid;
   END IF;
   IF coalesce(o,'')<>'' THEN
    PERFORM public.cast_private_subject_lock(o);
    PERFORM public.sytue_private_storage_guard('public.cast_private_storage_states'::regclass,t,o);
   END IF;
   FOR actor IN SELECT DISTINCT v FROM(VALUES(value->>'author_sub'),(value->>'reporter_sub'),(value->>'user_id'))actors(v) WHERE coalesce(v,'')<>'' ORDER BY v LOOP
    PERFORM public.cast_private_subject_guard(actor);
   END LOOP;
  END IF;
 END LOOP;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;RETURN NEW;
END $$;
CREATE TRIGGER cast_private_videos BEFORE INSERT OR UPDATE OR DELETE ON videos FOR EACH ROW EXECUTE FUNCTION cast_private_row_guard();
CREATE TRIGGER cast_private_people BEFORE INSERT OR UPDATE OR DELETE ON video_people FOR EACH ROW EXECUTE FUNCTION cast_private_row_guard();
CREATE TRIGGER cast_private_jobs BEFORE INSERT OR UPDATE OR DELETE ON transcode_jobs FOR EACH ROW EXECUTE FUNCTION cast_private_row_guard();
CREATE TRIGGER cast_private_comments BEFORE INSERT OR UPDATE OR DELETE ON comments FOR EACH ROW EXECUTE FUNCTION cast_private_row_guard();
CREATE TRIGGER cast_private_chat BEFORE INSERT OR UPDATE OR DELETE ON chat_messages FOR EACH ROW EXECUTE FUNCTION cast_private_row_guard();
CREATE TRIGGER cast_private_keys BEFORE INSERT OR UPDATE OR DELETE ON stream_keys FOR EACH ROW EXECUTE FUNCTION cast_private_row_guard();
CREATE TRIGGER cast_private_viewers BEFORE INSERT OR UPDATE OR DELETE ON live_viewers FOR EACH ROW EXECUTE FUNCTION cast_private_row_guard();
CREATE TRIGGER cast_private_presentations BEFORE INSERT OR UPDATE OR DELETE ON live_presentations FOR EACH ROW EXECUTE FUNCTION cast_private_row_guard();
CREATE TRIGGER cast_private_slides BEFORE INSERT OR UPDATE OR DELETE ON slide_marks FOR EACH ROW EXECUTE FUNCTION cast_private_row_guard();
CREATE TRIGGER cast_private_reports BEFORE INSERT OR UPDATE OR DELETE ON reports FOR EACH ROW EXECUTE FUNCTION cast_private_row_guard();
CREATE TRIGGER cast_private_blocks BEFORE INSERT OR UPDATE OR DELETE ON person_blocks FOR EACH ROW EXECUTE FUNCTION cast_private_row_guard();

-- Routes absent from the model must fail closed, not disappear from inventory.
-- Only booleans leave this maintenance lookup; foreign report identities stay
-- in their original moderation store and are never exposed by source pages.
CREATE FUNCTION cast_private_unmodeled(t text,o text) RETURNS boolean LANGUAGE sql STABLE
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
 SELECT EXISTS(SELECT 1 FROM public.chat_messages c JOIN public.videos v ON v.tenant=c.tenant AND v.id=c.video_id WHERE c.author_sub=o AND(v.tenant<>t OR v.owner_sub<>o))
 OR EXISTS(SELECT 1 FROM public.comments c JOIN public.videos v ON v.tenant=c.tenant AND v.id=c.video_id WHERE c.author_sub=o AND(v.tenant<>t OR v.owner_sub<>o))
 OR EXISTS(SELECT 1 FROM public.video_people p JOIN public.videos v ON v.tenant=p.tenant AND v.id=p.video_id WHERE p.user_id=o AND(v.tenant<>t OR v.owner_sub<>o))
 OR EXISTS(SELECT 1 FROM public.reports r WHERE r.reporter_sub=o AND NOT EXISTS(SELECT 1 FROM public.videos v WHERE v.tenant=r.tenant AND v.id=r.video_id AND v.tenant=t AND v.owner_sub=o))
 OR EXISTS(SELECT 1 FROM public.person_blocks b WHERE b.blocked_sub=o AND b.blocker_sub<>o)
 OR EXISTS(SELECT 1 FROM public.reports r JOIN public.videos v ON v.tenant=r.tenant AND v.id=r.video_id WHERE v.tenant=t AND v.owner_sub=o AND r.reporter_sub<>o)
 OR EXISTS(SELECT 1 FROM public.videos v WHERE v.tenant=t AND v.owner_sub=o AND v.kind='live')
 $$;

-- Existing private source identities may not be retagged by a generic merge.
-- sytue:scope-bound-merge-v1
SELECT sytue_scope_merge_register('cast_private_storage_snapshot'::regclass);
-- sytue:scope-admission-v1
SELECT sytue_scope_admission_register_existing();
