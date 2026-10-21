-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE org_profiles
    DROP COLUMN IF EXISTS cf_api_token,
    DROP COLUMN IF EXISTS cf_zone_id;
