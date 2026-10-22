-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE oauth_clients
    DROP CONSTRAINT IF EXISTS oauth_clients_impersonation_ck;

ALTER TABLE oauth_clients
    DROP COLUMN IF EXISTS may_impersonate_admins,
    DROP COLUMN IF EXISTS may_impersonate;
