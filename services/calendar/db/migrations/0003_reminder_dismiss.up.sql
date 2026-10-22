-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Entrega in-app de recordatorios: además del worker (que publica al bus para
-- canales externos), la UI sondea los recordatorios VENCIDOS del usuario y los
-- muestra en un centro de avisos. `dismissed_at` marca los que el usuario ya
-- descartó (independiente de `fired_at`, que es la publicación al bus): así el
-- aviso in-app y el del bus no compiten por la misma marca.

ALTER TABLE calendar_reminders ADD COLUMN dismissed_at timestamptz;

-- Cola in-app POR USUARIO: due (trigger_at<=now) y no descartados, en orden
-- keyset (trigger_at, id). Índice parcial (solo no descartados) que sirve el
-- WHERE (tenant, owner_sub, trigger_at) + el ORDER BY/keyset sin seq-scan.
CREATE INDEX calendar_reminders_inapp_idx
    ON calendar_reminders (tenant, owner_sub, trigger_at, id)
    WHERE dismissed_at IS NULL;
