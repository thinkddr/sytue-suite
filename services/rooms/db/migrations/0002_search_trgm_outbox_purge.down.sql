-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte los índices de búsqueda/purga. La extensión pg_trgm se conserva
-- (puede estar en uso por otros esquemas de la misma base).

DROP INDEX IF EXISTS events_outbox_created_idx;
DROP INDEX IF EXISTS messages_body_trgm;
