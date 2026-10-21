-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El esquema de Sytue Bases (`sytue-bases`) en el client de las apps MÓVILES (0071).
--
-- Sytue Bases tiene app nativa desde el 2026-10-05 (`apps/fyne/cmd/bases`) y, como todo
-- `cmd/` salvo la demo, se empaqueta para Android (`empaquetar.sh apps_moviles`): vuelve
-- del navegador del sistema por su esquema propio. Sin esta entrada Sytue ID contesta
-- 400 «redirect_uri is not registered for the client» y no se puede iniciar sesión en
-- un teléfono, que es lo que les pasó a Rooms y al Hub hasta la 0103. Lo vigila
-- `TestCadaEsquemaMovilEstaDadoDeAlta` (apps/fyne/sesion).
--
-- El escritorio no necesita nada: vuelve a un servidor efímero del equipo con el client
-- `sytue-desktop` (0069), que no lleva un redirect_uri por producto.
--
-- `array_append` solo si no está ya: la migración tiene que poder repetirse sin
-- duplicar entradas (pg.MigrateUp corre al arrancar cada réplica).
BEGIN;

UPDATE oauth_clients
SET redirect_uris = array_append(redirect_uris, 'sytue-bases://auth/callback')
WHERE id = 'sytue-mobile'
  AND NOT ('sytue-bases://auth/callback' = ANY (redirect_uris));

COMMIT;
