-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS outbound_scheduled_idx;
DROP INDEX IF EXISTS outbound_release_idx;
DROP INDEX IF EXISTS outbound_claim_idx;
CREATE INDEX outbound_claim_idx ON outbound (next_retry_at)
    WHERE state IN ('queued', 'deferred');

ALTER TABLE outbound DROP COLUMN IF EXISTS subject;
ALTER TABLE outbound DROP COLUMN IF EXISTS hold;
