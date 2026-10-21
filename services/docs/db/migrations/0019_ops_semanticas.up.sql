-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- OPERACIONES SEMÁNTICAS (parche.go): dos índices parciales sobre el log.
--   · op_id: la idempotencia de un parche mira si su op_id ya consta en el documento.
--   · registro vigente: aplicar un parche lee el último valor del registro de contenido
--     (doc:sheet / doc:slides) sin recorrer todo el historial del documento.
CREATE INDEX IF NOT EXISTS doc_operations_op_id_idx
    ON doc_operations (document_id, (op->>'op_id'))
    WHERE op ? 'op_id';

CREATE INDEX IF NOT EXISTS doc_operations_set_clave_idx
    ON doc_operations (document_id, (op->>'key'), lamport DESC)
    WHERE op->>'kind' = 'set';
