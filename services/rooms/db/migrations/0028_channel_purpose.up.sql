-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- PROPÓSITO DEL CANAL (paridad §4: «Propósito del canal … como campo distinto del tema»).
--
-- `topic` es la línea que se lee BAJO el nombre del canal, y por eso es corta y
-- cambia a menudo («desplegando la 2.4»). El PROPÓSITO responde a otra pregunta —«¿de
-- qué va este canal y me interesa entrar?»— y se lee en «Explorar canales», donde
-- hasta ahora se pintaba el tema por no haber otra cosa: quien exploraba veía el
-- estado del día en vez de para qué sirve el canal. Slack y Mattermost mantienen los
-- dos campos separados por exactamente esa razón.
--
-- ADD COLUMN ... DEFAULT constante lo resuelve Postgres por metadatos, sin reescribir
-- la tabla (barato a escala). Expand puro: la versión anterior del servicio ignora la
-- columna y sigue funcionando.

ALTER TABLE channels ADD COLUMN purpose text NOT NULL DEFAULT '';
