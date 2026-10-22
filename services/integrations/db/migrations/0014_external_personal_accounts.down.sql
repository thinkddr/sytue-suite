-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM app_instances WHERE external_chat_actor<>'') THEN
        RAISE EXCEPTION 'Personal account credentials must be disconnected before rollback';
    END IF;
END $$;
ALTER TABLE app_instances DROP COLUMN external_chat_actor;
