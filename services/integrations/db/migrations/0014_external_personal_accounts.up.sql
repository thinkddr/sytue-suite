-- SPDX-License-Identifier: Apache-2.0
-- A personal external chat credential is never delegated by the org reader ACL.
ALTER TABLE app_instances ADD COLUMN external_chat_actor text NOT NULL DEFAULT '';
CREATE INDEX app_instances_external_actor ON app_instances(org_id,external_chat_actor)
    WHERE external_chat_actor<>'';
