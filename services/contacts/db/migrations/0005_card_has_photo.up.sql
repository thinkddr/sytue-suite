-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- `has_photo`: si la tarjeta lleva una PHOTO EMBEBIDA (data URI de imagen), que es
-- justo lo que sirve `GET /api/v1/contacts/photo`.
--
-- POR QUÉ. La web sabe qué contactos tienen foto porque lista por CardDAV y ve la
-- PHOTO en cada vCard (`hasEmbeddedPhoto`); la API REST no lo decía, así que la app
-- nativa pintaba iniciales en toda la lista: pedir la foto fila a fila serían miles
-- de 404. Con esto el listado lo dice y solo se pide la que existe.
--
-- COLUMNA GENERADA, y no un campo que mantenga el código: la tarjeta se escribe
-- por cinco caminos (REST, CardDAV, foto, reflejo del avatar de Sytue ID, importar)
-- y a una columna generada no se le olvida ninguno. Se calcula al ESCRIBIR, una vez;
-- leerla en el listado no toca `data`, que con fotos pesa cientos de KiB por fila.
--
-- La regla es la de `hasEmbeddedPhoto` de la web, literal: se desdoblan las líneas
-- plegadas (RFC 6350 §3.2) y se busca una propiedad PHOTO —con o sin grupo,
-- `item1.PHOTO`— cuyo valor sea `data:image/…`. Una PHOTO que es una URL no cuenta:
-- esa no la sirve el servicio.
ALTER TABLE card_objects
    ADD COLUMN has_photo boolean NOT NULL GENERATED ALWAYS AS (
        regexp_replace(data, '\r?\n[ \t]', '', 'g')
            ~* '(^|\n)([^\n:;]*\.)?PHOTO(;[^\n:]*)?:([^\n]*:)?[ \t]*data:image/'
    ) STORED;
