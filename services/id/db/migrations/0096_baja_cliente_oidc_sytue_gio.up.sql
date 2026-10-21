-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Baja del client OIDC de la suite en Gio (`sytue-gio`, dado de alta en la 0070).
-- Su BFF (services/gioweb) se retiró del repositorio el 2026-09-23 y `sytue.com`
-- es un 301 a sytue.eu, así que nadie pide ya tokens con él; lo decidió el dueño.
--
-- NUEVA y no una 0070 reescrita: la 0070 ya está aplicada en producción y
-- golang-migrate no vuelve a leerla, así que editarla no quitaría la fila en
-- ninguna base existente (ver la 0023 de Drive).
--
-- Los códigos y refresh tokens que quedaran de ese client se van con él
-- (`auth_codes` y `refresh_tokens` lo referencian con ON DELETE CASCADE): una
-- sesión abierta en `gio.sytue.eu` deja de poder renovarse, que es lo que se
-- quiere. Sin la fila, el barrido de cierre de sesión deja de pedirle a
-- `gio.sytue.eu` su logout, y por eso en el mismo cambio salen de
-- platform/pkg/config la audiencia y los orígenes de la CSP.
--
-- Idempotente: `pg.MigrateUp` corre al arrancar cada réplica.
BEGIN;

DELETE FROM oauth_clients WHERE id = 'sytue-gio';

COMMIT;
