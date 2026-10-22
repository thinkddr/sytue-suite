-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Consulta admin del log de auditoría (Console): SIEMPRE acotada a una
-- organización (org_id) y paginada por keyset (occurred_at, id) DESC. El índice
-- 0006 (solo occurred_at) no sirve el filtro por org como Index Scan, así que a
-- escala forzaría un Bitmap/Seq Scan + Sort sobre una tabla append-only que solo
-- crece. Este índice compuesto sirve el WHERE org_id = $1 + el ORDER BY/keyset
-- (occurred_at, id) DESC de una sola pasada, sin OFFSET.

CREATE INDEX IF NOT EXISTS audit_log_org_occurred_idx
    ON audit_log (org_id, occurred_at DESC, id DESC);
