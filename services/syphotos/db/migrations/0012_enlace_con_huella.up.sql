-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- EL ENLACE PÚBLICO DE UN ÁLBUM SE GUARDA COMO HUELLA (FILE-10, decidido por el
-- dueño el 2026-10-15: «uno nuevo y el anterior caduca»).
--
-- El token es la credencial del enlace y estaba en claro: quien leyera la base
-- (una copia de seguridad, un volcado) abría cualquier álbum compartido. Ahora se
-- guarda su SHA-256 en hexadecimal; el token solo existe en la respuesta de
-- «Compartir», y pulsarlo otra vez genera uno nuevo.
--
-- Los enlaces que ya se mandaron SIGUEN VALIENDO: su huella se calcula aquí del
-- token en claro, que se borra en el mismo paso.
ALTER TABLE albums ADD COLUMN public_token_hash text NOT NULL DEFAULT '';

UPDATE albums
   SET public_token_hash = encode(sha256(convert_to(public_token, 'UTF8')), 'hex'),
       public_token = ''
 WHERE public_token <> '';

CREATE UNIQUE INDEX albums_public_token_hash ON albums (public_token_hash) WHERE public_token_hash <> '';

-- La puerta del enlace (0004) mira ahora la huella: con el token en claro vacío,
-- la anterior ya no abría nada.
DROP POLICY IF EXISTS albums_enlace_publico ON albums;
CREATE POLICY albums_enlace_publico ON albums
    FOR SELECT
    USING (
        current_setting('app.public_link', true) = 'on'
        AND length(public_token_hash) > 0
    );
