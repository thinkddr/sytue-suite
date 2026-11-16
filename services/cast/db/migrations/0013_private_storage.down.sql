-- SPDX-License-Identifier: Apache-2.0
-- A retained capture is not disposable migration machinery.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM cast_private_storage_states WHERE mode<>'legacy')
 OR EXISTS(SELECT 1 FROM cast_private_storage_snapshot)
 OR EXISTS(SELECT 1 FROM videos WHERE visibility='private') THEN
  RAISE EXCEPTION 'Cast private sources must remain retained' USING ERRCODE='PST01';
 END IF;
END $$;
SELECT sytue_scope_admission_remove();
DROP TABLE private_scope_admission_control;
DROP FUNCTION sytue_scope_admission_remove();
DROP FUNCTION sytue_scope_admission_register_existing();
DROP FUNCTION sytue_scope_admission_tables();
DROP FUNCTION sytue_scope_admission_transition(uuid,text,text,text);
DROP FUNCTION sytue_scope_admission_register(regclass,boolean);
DROP FUNCTION sytue_scope_admission_data_guard();
DROP FUNCTION sytue_scope_admission_check(text,boolean);
DROP FUNCTION sytue_scope_admission_control_guard();
SELECT sytue_scope_merge_unregister(ARRAY['cast_private_storage_snapshot'::regclass]);
DROP TRIGGER cast_private_videos ON videos;
DROP TRIGGER cast_private_people ON video_people;
DROP TRIGGER cast_private_jobs ON transcode_jobs;
DROP TRIGGER cast_private_comments ON comments;
DROP TRIGGER cast_private_chat ON chat_messages;
DROP TRIGGER cast_private_keys ON stream_keys;
DROP TRIGGER cast_private_viewers ON live_viewers;
DROP TRIGGER cast_private_presentations ON live_presentations;
DROP TRIGGER cast_private_slides ON slide_marks;
DROP TRIGGER cast_private_reports ON reports;
DROP TRIGGER cast_private_blocks ON person_blocks;
DROP FUNCTION cast_private_row_guard();
DROP FUNCTION cast_private_unmodeled(text,text);
DROP FUNCTION cast_private_subject_guard(text);
DROP FUNCTION cast_private_subject_freeze(text);
DROP FUNCTION cast_private_subject_lock(text);
DROP TABLE cast_private_storage_snapshot;
DROP FUNCTION cast_private_snapshot_insert_guard();
DROP TABLE cast_private_storage_states;
DROP FUNCTION cast_private_retained_truncate();
ALTER TABLE videos DROP CONSTRAINT videos_visibility_check;
ALTER TABLE videos ADD CONSTRAINT videos_visibility_check CHECK(visibility IN('public','unlisted','org','people'));
