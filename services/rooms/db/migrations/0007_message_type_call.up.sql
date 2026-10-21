-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Tipo de mensaje: "user" (mensaje normal) o "call" (tarjeta de sistema que anota
-- el inicio de una videollamada de Sytue Rooms en el timeline del canal — Ola 6
-- «Llamadas ↔ chat»). El resto de mensajes de sistema futuros reutilizan esta
-- columna sin nuevas migraciones.
--
-- Backfill: ADD COLUMN ... NOT NULL DEFAULT 'user' fija 'user' en todas las filas
-- existentes. Con un DEFAULT constante Postgres (>=11) lo resuelve por metadatos,
-- sin reescribir la tabla — barato a escala (cientos de miles de mensajes).
--
-- RLS: la tabla messages ya tiene ENABLE/FORCE ROW LEVEL SECURITY y la política de
-- aislamiento por tenant (0001_init); una columna nueva la hereda sin tocar nada.

ALTER TABLE messages ADD COLUMN type text NOT NULL DEFAULT 'user';

ALTER TABLE messages ADD CONSTRAINT messages_type_valid CHECK (type IN ('user', 'call'));

-- El CHECK original exigía cuerpo no vacío SIEMPRE. Un mensaje de sistema (call) y
-- uno solo-adjuntos llevan cuerpo vacío legítimamente: relajamos el invariante a
-- «un mensaje de USUARIO sin adjuntos debe tener cuerpo». Solo afloja; nunca es más
-- estricto que el anterior.
ALTER TABLE messages DROP CONSTRAINT messages_body_not_blank;
ALTER TABLE messages ADD CONSTRAINT messages_body_not_blank
    CHECK (length(body) > 0 OR type <> 'user' OR attachments <> '[]'::jsonb);
