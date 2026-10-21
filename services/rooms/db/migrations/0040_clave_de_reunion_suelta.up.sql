-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- LA CLAVE DEL ENLACE DE UNA REUNIÓN SUELTA (decidido por el dueño el 2026-10-16:
-- «Invitados y quien tenga el enlace»).
--
-- Una reunión que no cuelga de un canal ni de un documento la abría cualquiera de la
-- organización que conociera su id, y GET /rooms se los daba todos. Ahora entran quien
-- la convocó, quien ya estuvo dentro y quien trae el enlace, que lleva esta clave: más de 240
-- bits que no salen del id. Las salas que ya existen reciben la suya aquí, así que sus
-- enlaces viejos (sin clave) dejan de abrir la puerta a quien no estuvo dentro.
--
-- gen_random_uuid() sale del generador criptográfico de PostgreSQL (13+): dos son 244
-- bits aleatorios.
ALTER TABLE rooms ADD COLUMN clave text NOT NULL
    DEFAULT replace(gen_random_uuid()::text || gen_random_uuid()::text, '-', '');
