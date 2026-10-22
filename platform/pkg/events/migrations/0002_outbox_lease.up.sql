-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Reserva (lease) del outbox. El Dispatcher pasa a ejecutar el Handler FUERA de
-- la transacción del lote: mantenerlo dentro provocaba un deadlock de pool (la
-- tx retiene una conexión y el handler pide otra) y sostenía locks de fila
-- durante el I/O de red del handler.
--
-- Sin el lock de fila haciendo de exclusión mutua, es `locked_until` quien evita
-- que el poll de respaldo, u otra réplica, reclame un evento en curso.

ALTER TABLE events_outbox ADD COLUMN IF NOT EXISTS locked_until timestamptz;
