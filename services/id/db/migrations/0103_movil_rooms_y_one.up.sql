-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Los esquemas de Sytue Rooms (`sytue-rooms`) y de la app de la suite (`sytue-one`,
-- el Hub) en el client de las apps MÓVILES (0071).
--
-- Las dos se empaquetan para Android (`empaquetar.sh apps_moviles`: todo `cmd/` menos
-- la demo) y vuelven del navegador del sistema por su esquema propio, pero 0071 se
-- escribió con la lista de las apps de Gio y 0100 solo añadió el de Cast: Sytue ID
-- contestaba 400 «redirect_uri is not registered for the client» y ninguna de las dos
-- podía iniciar sesión en un teléfono (medido contra id.sytue.eu el 2026-09-28). Lo
-- vigila `TestCadaEsquemaMovilEstaDadoDeAlta` (apps/fyne/sesion).
--
-- `array_append` solo si no está ya: la migración tiene que poder repetirse sin
-- duplicar entradas (pg.MigrateUp corre al arrancar cada réplica).
BEGIN;

UPDATE oauth_clients
SET redirect_uris = array_append(redirect_uris, 'sytue-rooms://auth/callback')
WHERE id = 'sytue-mobile'
  AND NOT ('sytue-rooms://auth/callback' = ANY (redirect_uris));

UPDATE oauth_clients
SET redirect_uris = array_append(redirect_uris, 'sytue-one://auth/callback')
WHERE id = 'sytue-mobile'
  AND NOT ('sytue-one://auth/callback' = ANY (redirect_uris));

COMMIT;
