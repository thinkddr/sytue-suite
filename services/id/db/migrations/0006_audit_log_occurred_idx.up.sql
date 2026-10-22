-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- audit_log solo tenía índices por actor_id/org_id (0001): cualquier consulta
-- o informe por rango de fechas hacía un Seq Scan sobre una tabla append-only
-- que solo crece. Índice temporal para los WHERE/ORDER BY por occurred_at.

CREATE INDEX IF NOT EXISTS audit_log_occurred_idx ON audit_log (occurred_at);
