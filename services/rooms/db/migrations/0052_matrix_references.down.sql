-- SPDX-License-Identifier: Apache-2.0
-- Encrypted references cannot be converted to plaintext on rollback.
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM messages WHERE matrix_event_id<>'') OR EXISTS(SELECT 1 FROM matrix_identity_access WHERE version>0) OR EXISTS(SELECT 1 FROM matrix_tenant_access WHERE version>0) THEN
        RAISE EXCEPTION 'Encrypted message references must be preserved before rollback';
    END IF;
END $$;
ALTER TABLE messages DROP CONSTRAINT messages_body_not_blank;
ALTER TABLE messages DROP COLUMN matrix_room_id, DROP COLUMN matrix_event_id;
ALTER TABLE messages ADD CONSTRAINT messages_body_not_blank
    CHECK (length(body)>0 OR type<>'user' OR attachments<>'[]'::jsonb);
DROP TABLE matrix_channel_rooms;
DROP FUNCTION platform_merge_prepare(text,text);
DROP TABLE matrix_tenant_access;

DROP TABLE matrix_identity_access;
