-- SPDX-License-Identifier: Apache-2.0
DELETE FROM whatsapp_accounts WHERE NOT ready;
ALTER TABLE whatsapp_accounts DROP COLUMN instance_id;
ALTER TABLE whatsapp_accounts DROP COLUMN ready;
