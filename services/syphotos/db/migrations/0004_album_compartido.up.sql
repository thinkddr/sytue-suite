-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- ÁLBUM COMPARTIDO por enlace.
--
-- El token vive en el propio álbum y no en una tabla de comparticiones: hay
-- como mucho UNO por álbum y no lleva estado propio —ni destinatarios, ni
-- caducidad, ni permisos—. Una tabla aparte solo haría falta el día que haya
-- varios enlaces por álbum o uno por persona, y entonces se migra.
ALTER TABLE albums ADD COLUMN public_token text NOT NULL DEFAULT '';

-- Único en TODA la instalación: es la credencial, y dos álbumes con el mismo
-- token serían dos puertas con la misma llave.
CREATE UNIQUE INDEX albums_public_token ON albums (public_token) WHERE public_token <> '';

-- Política PERMISSIVE acotada a `app.public_link='on'`.
--
-- Un álbum compartido se abre SIN sesión, así que en esa petición no hay
-- organización que fijar: `app.tenant` viene vacío, la política de aislamiento
-- —`tenant = current_setting('app.tenant', true)`— evalúa a NULL y no devuelve
-- NI UNA fila, ni siquiera la del token correcto. Es la degradación correcta de
-- RLS y también el motivo por el que, sin esto, el enlace diría «no existe»
-- para todos los tokens, incluidos los buenos.
--
-- El alcance es deliberadamente estrecho, en tres verjas:
--   · Variable propia `app.public_link`: ninguna transacción existente gana
--     alcance nuevo por esto.
--   · La fija UNA sola función del store, la que resuelve el token.
--   · Y aun con la variable puesta, la política solo ve álbumes CON token; los
--     32 bytes aleatorios siguen siendo la credencial.
--
-- Solo `albums`. Las fotos del álbum se leen DESPUÉS, ya sabiendo a qué
-- organización pertenece, con `app.tenant` fijado como cualquier otra consulta:
-- ahí la política de aislamiento normal basta y no hay que relajar nada.
DROP POLICY IF EXISTS albums_enlace_publico ON albums;
CREATE POLICY albums_enlace_publico ON albums
    FOR SELECT
    USING (
        current_setting('app.public_link', true) = 'on'
        AND length(public_token) > 0
    );
