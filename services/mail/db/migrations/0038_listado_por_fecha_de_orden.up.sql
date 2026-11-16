-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- El índice del listado de carpeta por la FECHA DE ORDEN de la 0037,
-- COALESCE(unsnoozed_at, received_at), para que el cursor keyset siga siendo O(1) por
-- página. CONCURRENTLY para no bloquear las escrituras de `messages` mientras se
-- construye (en una tabla grande, minutos); por eso va solo en su fichero: golang-migrate
-- manda el fichero entero en una llamada y varias sentencias juntas irían en una
-- transacción implícita, que CONCURRENTLY no admite.
--
-- Si se corta a medias, el índice queda INVÁLIDO y la migración «dirty»: relanzar no lo
-- arregla (IF NOT EXISTS lo da por hecho). Receta en docs/operations.md, junto a la de
-- la 0008 del Hub (indisvalid → DROP INDEX CONCURRENTLY → migrate force 37).
--
-- Hasta que exista, las listas siguen funcionando (más lentas): el orden lo pone la
-- consulta, no el índice.
CREATE INDEX CONCURRENTLY IF NOT EXISTS messages_mailbox_orden_idx
    ON messages (tenant, mailbox_id, (COALESCE(unsnoozed_at, received_at)) DESC)
    WHERE deleted_at IS NULL;
