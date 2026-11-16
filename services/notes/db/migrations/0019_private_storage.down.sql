-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM notes_private_storage_states WHERE mode<>'legacy') THEN
  RAISE EXCEPTION 'private Notes migration cannot be downgraded after a source is frozen';
 END IF;
END $$;
DROP TRIGGER notes_private_notebooks ON notebooks;
DROP TRIGGER notes_private_notes ON notes;
DROP TRIGGER notes_private_tags ON tags;
DROP TRIGGER notes_private_tag_states ON tag_states;
DROP TRIGGER notes_private_marks ON note_tag_states;
DROP TRIGGER notes_private_links ON note_links;
DROP TRIGGER notes_private_reminders ON note_reminders;
DROP TRIGGER notes_private_readers ON notebook_lectores;
DROP TRIGGER notes_private_editors ON notebook_editores;
DROP TRIGGER notes_private_requests ON notebook_peticiones_edicion;
DROP TRIGGER notes_private_people ON note_personas;
DROP FUNCTION notes_private_row_guard();
DROP FUNCTION notes_private_row_owners(text,jsonb);
DROP TABLE notes_private_storage_snapshot;
DROP TABLE notes_private_storage_states;
