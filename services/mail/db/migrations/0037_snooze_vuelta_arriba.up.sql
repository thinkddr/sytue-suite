-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- LO POSPUESTO VUELVE ARRIBA, como en Gmail. Hasta 0036 un hilo que volvía de
-- «Pospuestos» al vencer ocupaba su sitio cronológico en Recibidos (el de su fecha
-- de llegada), varias páginas más abajo: posponer servía para olvidarse de él, no
-- para que apareciera cuando tocaba.
--
-- `unsnoozed_at` es cuándo volvió el mensaje de «Pospuestos» al vencer la fecha (la
-- del posponer, no la del barrido). Las listas ordenan por la FECHA DE ORDEN,
-- COALESCE(unsnoozed_at, received_at): lo que no se pospuso nunca sigue en su sitio
-- y lo devuelto sube a la hora de vuelta. `received_at` no se toca: es la fecha de
-- llegada que enseña el lector, la que usan IMAP, la búsqueda por fechas y el
-- e-discovery.
--
-- Columna NULLable sin DEFAULT: añadirla no reescribe la tabla.
--
-- El índice del listado de carpeta pasa a ordenar por la fecha de orden (expresión),
-- para que el cursor keyset siga siendo O(1) por página: sustituye al de 0001, que
-- ordenaba por received_at y ya no lo usaría ninguna consulta de lista.

ALTER TABLE messages ADD COLUMN unsnoozed_at timestamptz;

CREATE INDEX messages_mailbox_orden_idx
    ON messages (tenant, mailbox_id, (COALESCE(unsnoozed_at, received_at)) DESC)
    WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS messages_mailbox_idx;
