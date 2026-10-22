-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Envío diferido (programado + deshacer) sobre la MISMA cola de salida.
-- `next_retry_at` ya es el instante de liberación que el claim respeta
-- (state IN ('queued','deferred') AND next_retry_at <= now(), FOR UPDATE SKIP
-- LOCKED): un mensaje diferido no es más que una fila 'queued' con
-- next_retry_at en el futuro. `hold` distingue esas filas «retenidas» (aún no
-- son una entrega, sino una submission pendiente que el worker LIBERA al vencer)
-- de las filas de entrega normales (hold=''):
--   ''         → entrega inmediata / reintento (las drena ClaimDue como siempre).
--   'scheduled'→ programado por el usuario (aparece en la vista «Programados»).
--   'undo'     → ventana de deshacer (transitorio; no se lista).
-- `subject` se desnormaliza para pintar la lista de programados sin bajar el .eml.
ALTER TABLE outbound
    ADD COLUMN hold    text NOT NULL DEFAULT ''
        CHECK (hold IN ('', 'scheduled', 'undo')),
    ADD COLUMN subject text NOT NULL DEFAULT '';

-- El claim de ENTREGA excluye las filas retenidas (las libera PromoteDueHeld).
DROP INDEX outbound_claim_idx;
CREATE INDEX outbound_claim_idx ON outbound (next_retry_at)
    WHERE state IN ('queued', 'deferred') AND hold = '';

-- El claim de LIBERACIÓN de diferidos vencidos (worker cross-réplica, SKIP LOCKED).
CREATE INDEX outbound_release_idx ON outbound (next_retry_at)
    WHERE state = 'queued' AND hold <> '';

-- La vista «Programados» de una cuenta, paginada por keyset (next_retry_at, id).
CREATE INDEX outbound_scheduled_idx ON outbound (tenant, account_id, next_retry_at, id)
    WHERE state = 'queued' AND hold = 'scheduled';
