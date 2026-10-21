-- SPDX-License-Identifier: Apache-2.0
-- Meta verifies the callback before the guided setup can become active.
-- Existing manually connected numbers remain active.
ALTER TABLE whatsapp_accounts ADD COLUMN ready boolean NOT NULL DEFAULT true;
ALTER TABLE whatsapp_accounts ADD COLUMN instance_id text NOT NULL DEFAULT '';
