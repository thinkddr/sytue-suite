-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Los salientes con dialecto 'teams' vuelven al POST nativo: la restricción no los
-- admitiría y dejarlos rotos sería peor que degradarlos.
UPDATE outgoing_webhooks SET dialect = 'sytue' WHERE dialect = 'teams';

ALTER TABLE outgoing_webhooks
    DROP CONSTRAINT outgoing_webhooks_dialect_known,
    ADD CONSTRAINT outgoing_webhooks_dialect_known CHECK (dialect IN ('sytue', 'slack'));

ALTER TABLE outgoing_webhooks DROP COLUMN signing_secret;
