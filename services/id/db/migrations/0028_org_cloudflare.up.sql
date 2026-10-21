-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- «Mi dominio está en Cloudflare, configúralo por mí»: el cliente pega un token de
-- SU cuenta de Cloudflare y Sytue le crea los registros que necesita.
--
-- Ese token es un secreto DE UN TERCERO, no nuestro, y por defecto NO se guarda:
-- se usa y se descarta en la misma petición. Estas dos columnas existen solo para
-- el caso en que el cliente pide expresamente recordarlo («no me lo vuelvas a
-- pedir»), y entonces:
--
--   * cf_api_token se guarda SELLADO con la KEK del servicio (keycrypto,
--     AES-256-GCM, "encv1:…"), igual que la clave privada de firma y los secretos
--     de federación. Nunca en claro, nunca en un log, nunca hacia el navegador.
--   * cf_zone_id es un identificador público de Cloudflare (no es un secreto):
--     se guarda para no tener que volver a buscar la zona.
--
-- Van en org_profiles porque son configuración del DOMINIO PROPIO de la
-- organización, que ya vive ahí (mig 0027). NO entran en la proyección que
-- serializa el perfil: se leen con su propia consulta, para que un olvido no las
-- pueda colar en una respuesta HTTP.
--
-- Borrarlas es un UPDATE a '' (el asistente lo hace en cuanto el cliente desmarca
-- «recordar»), y la baja de la organización se las lleva por CASCADE.

ALTER TABLE org_profiles
    ADD COLUMN cf_zone_id   text NOT NULL DEFAULT '',
    ADD COLUMN cf_api_token text NOT NULL DEFAULT '';
