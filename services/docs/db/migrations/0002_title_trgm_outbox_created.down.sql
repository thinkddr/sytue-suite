-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS events_outbox_created_idx;

DROP INDEX IF EXISTS documents_title_trgm_idx;
CREATE INDEX documents_title_trgm_idx ON documents (tenant, lower(title));

-- pg_trgm se conserva: es inocua y otros índices podrían depender de ella.
