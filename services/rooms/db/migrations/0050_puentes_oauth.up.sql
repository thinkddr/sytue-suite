-- SPDX-License-Identifier: Apache-2.0
-- OAuth credentials remain in Integrations. Existing webhook bridges keep working.
ALTER TABLE channel_bridges
    ADD COLUMN instance_id text NOT NULL DEFAULT '',
    ADD COLUMN remote_id text NOT NULL DEFAULT '';
ALTER TABLE channel_bridges ADD CONSTRAINT channel_bridges_oauth_target
    CHECK ((instance_id = '' AND remote_id = '') OR
           (instance_id <> '' AND remote_id <> '' AND webhook_url = ''));
