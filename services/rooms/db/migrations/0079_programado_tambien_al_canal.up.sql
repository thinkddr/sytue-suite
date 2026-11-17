-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- «TAMBIÉN ENVIAR AL CANAL» en una respuesta PROGRAMADA (mig. 0078 para las que salen ya):
-- la cola de programados no guardaba la casilla, y la respuesta que su autor quiso ver
-- también en el canal se publicaba solo en el hilo al vencer. Solo vale con parent_id; el
-- servicio lo fuerza a false en una raíz.
--
-- DEFAULT constante: desde PG 11 el ADD COLUMN es solo metadatos, sin reescribir.
ALTER TABLE scheduled_messages ADD COLUMN also_in_channel boolean NOT NULL DEFAULT false;
