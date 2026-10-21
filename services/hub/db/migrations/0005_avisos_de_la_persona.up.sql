-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Un AVISO es de la PERSONA, no de la organización en la que estaba al recibirlo
-- (ADR 0053). Quien comenta en un documento o escribe en una conversación de otra
-- organización (ADR 0040, 0052) recibe el aviso en la anfitriona, y su campana
-- —con el token de SU organización— no lo veía.
--
-- Política PERMISSIVE que se suma al aislamiento por organización, SOLO para leer
-- y marcar como leído, y solo las filas cuyo destinatario es uno de los buzones de
-- `app.destinatarios`. Esa variable la fija el servicio en cada transacción con los
-- buzones PERSONALES del token verificado de quien llama (su `sub` y su `mailbox`;
-- el correo de entrar no, ver ADR 0053), nunca con nada que venga de la petición.
-- La política es por FILA, no por columna: hoy el servicio solo escribe `read` y
-- `read_at`, y un UPDATE nuevo sobre estas filas tiene que seguir así. Sin ella, `string_to_array(NULL, …)` es NULL y
-- la política no deja ver nada.
--
-- No hay INSERT ni DELETE nuevos: crear avisos sigue siendo en la organización del
-- emisor, y la purga sigue siendo la de mantenimiento.
CREATE POLICY notifications_destinatario_lee ON notifications
    FOR SELECT
    USING (subject = ANY (string_to_array(nullif(current_setting('app.destinatarios', true), ''), chr(31))));

CREATE POLICY notifications_destinatario_marca ON notifications
    FOR UPDATE
    USING (subject = ANY (string_to_array(nullif(current_setting('app.destinatarios', true), ''), chr(31))))
    WITH CHECK (subject = ANY (string_to_array(nullif(current_setting('app.destinatarios', true), ''), chr(31))));
