-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
DROP INDEX app_passwords_org_user_idx;
ALTER TABLE app_passwords DROP COLUMN org_id;
