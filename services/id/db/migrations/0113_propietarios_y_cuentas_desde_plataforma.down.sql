-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
ALTER TABLE org_invitations DROP COLUMN IF EXISTS invitee_name;
ALTER TABLE users DROP COLUMN IF EXISTS password_must_change;
