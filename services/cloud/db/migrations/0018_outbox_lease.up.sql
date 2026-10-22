-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Reserva (lease) del outbox. El Dispatcher pasa a ejecutar el Handler FUERA de
-- la transacción del lote: tenerlo dentro provocaba un deadlock de pool (la tx
-- retiene una conexión y el handler, vía store, pide otra) que dejaba la
-- conexión en «idle in transaction» y tumbaba el servicio tras el primer evento.
--
-- Sin el lock de fila haciendo de exclusión mutua, es `locked_until` quien evita
-- que el poll de respaldo, u otra réplica, reclame un evento aún en curso.

ALTER TABLE events_outbox ADD COLUMN IF NOT EXISTS locked_until timestamptz;
