-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE outgoing_webhooks
    DROP CONSTRAINT IF EXISTS outgoing_webhooks_dialect_known,
    DROP COLUMN IF EXISTS dialect;
