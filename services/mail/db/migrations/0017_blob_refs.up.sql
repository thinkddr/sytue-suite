-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Deduplicación de mensajes ENTRE organizaciones.
--
-- Un correo se guardaba una vez por destinatario: el mismo boletín en cien
-- buzones eran cien objetos idénticos en S3, y si esos buzones eran de cincuenta
-- organizaciones, cincuenta veces cincuenta. Ahora el .eml crudo se guarda UNA
-- vez bajo una clave direccionada por su SHA-256 (`shared/mensajes/<hash>.eml`,
-- ver objectstore.PrefijoCompartido) y cada fila de `messages` que lo referencia
-- cuenta una referencia aquí. Al purgar físicamente una fila la referencia baja,
-- y el objeto solo se borra cuando llega a cero.
--
-- SIN RLS, Y A PROPÓSITO: no hay columna tenant porque la tabla no tiene datos
-- de nadie —un hash, una clave y un contador—. Saber que un hash existe no
-- revela nada que no se supiera ya teniendo el mensaje, y el objeto solo se
-- alcanza a través de una fila de `messages`, que sí está aislada. El test de
-- RLS forzada no la exige porque no lleva columna de organización.

CREATE TABLE blob_refs (
    hash        text        PRIMARY KEY,
    key         text        NOT NULL UNIQUE,
    size_bytes  bigint      NOT NULL CHECK (size_bytes >= 0),
    refs        integer     NOT NULL DEFAULT 0 CHECK (refs >= 0),
    -- listo = el objeto ya está subido entero. Quien encuentra la fila sin
    -- `listo` vuelve a subir los mismos bytes (idempotente) en vez de apuntar a
    -- un objeto que otro proceso aún está escribiendo.
    listo       boolean     NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now()
);
