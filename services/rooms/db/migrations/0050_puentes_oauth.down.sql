-- SPDX-License-Identifier: Apache-2.0
-- Remove OAuth bridges before removing their destination metadata.
DELETE FROM channel_bridges WHERE instance_id <> '';
ALTER TABLE channel_bridges DROP CONSTRAINT channel_bridges_oauth_target;
ALTER TABLE channel_bridges DROP COLUMN remote_id, DROP COLUMN instance_id;
