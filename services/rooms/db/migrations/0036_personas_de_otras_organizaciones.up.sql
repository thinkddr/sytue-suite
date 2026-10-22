-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- PERSONAS DE OTRA ORGANIZACIÓN en una conversación de esta (ADR 0040): un DM, un
-- grupo o un canal abierto escribiendo el correo o el @usuario de alguien de fuera.
--
-- LA CONVERSACIÓN VIVE EN LA ORGANIZACIÓN DE QUIEN LA ABRE, con sus filas, su RLS
-- y su retención de siempre. Quien viene de fuera es una fila más de
-- channel_members con `externo`: no está en ningún equipo de aquí, así que las
-- verjas que exigen el equipo le dejan pasar por esa marca y solo en ese canal.
--
-- Y TIENE QUE PODER ENCONTRARLA DESDE LA SUYA. Su token trae su organización, no
-- esta, así que para listar sus conversaciones de fuera —y para saber a qué
-- organización pertenece el canal que abre— hace falta una búsqueda por persona
-- entre organizaciones. Va por la política de mantenimiento, como los webhooks
-- entrantes, pero SOLO para lectura y SOLO sobre las filas `externo`: una
-- transacción de mantenimiento no ve de channel_members nada más que eso.
--
-- Expand puro: columna con DEFAULT, ninguna fila existente cambia.

ALTER TABLE channel_members ADD COLUMN externo boolean NOT NULL DEFAULT false;

-- La búsqueda de sus conversaciones desde la organización de la persona. Parcial:
-- en casi toda la tabla la marca es false y no se pregunta nunca por esas filas.
CREATE INDEX channel_members_externos ON channel_members (subject) WHERE externo;

CREATE POLICY channel_members_externos_maintenance ON channel_members FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on' AND externo);
