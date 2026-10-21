-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Se retira el client del BFF de la app conjunta de Sytue Docs (`sytue-docs`).
--
-- Desde el 2026-10-12 (ADR 0076) Docus, Calcus, Diapos y CertiPDF tienen app y client
-- propios (0111) y `docs.<dominio>/docs` desaparece SIN redirecciones, por decisión del
-- dueño. Ningún despliegue anuncia ya las callbacks de este client, y dejarlas
-- registradas tendría dos costes: el barrido de cierre de sesión seguiría pidiendo un
-- `/auth/logout` a un host sin web, y cualquiera podría seguir iniciando un login
-- contra una dirección que ya no es de nadie.
--
-- Se BORRA y no se vacía: sus códigos, sesiones y refresh tokens caen con él (ON
-- DELETE CASCADE de 0001), que es justo lo que se quiere de un BFF que ya no existe.
-- La AUDIENCIA `sytue-docs` sigue en la lista de los servicios (platform/pkg/config):
-- es la del servicio de documentos, no la de este client.
-- sytue:destructive-ok el client de la app conjunta de Docs, retirada en esta versión (ADR 0076)
BEGIN;
DELETE FROM oauth_clients WHERE id = 'sytue-docs';
COMMIT;
