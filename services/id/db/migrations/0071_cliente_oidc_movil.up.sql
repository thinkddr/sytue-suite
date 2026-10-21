-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El client OIDC de las apps MÓVILES en Gio (`sytue-mobile`): la misma suite que
-- el escritorio y la web, empaquetada con gogio para Android e iOS (ADR 0045).
--
-- PÚBLICO y sin secreto, como `sytue-desktop` (0069) y al revés que `sytue-gio`
-- (0070): una app instalada en un teléfono no puede guardar un secreto, así que
-- lo que la protege es PKCE (`require_pkce`) más la validación EXACTA del
-- redirect. `token_endpoint_auth_method = 'none'` lo dice explícito.
--
-- POR QUÉ NO VALE EL LOOPBACK DEL ESCRITORIO. `sytue-desktop` vuelve a
-- `http://127.0.0.1:8921/auth/callback` porque ahí hay un servidor efímero en el
-- propio equipo (RFC 8252 §7.3). En un teléfono ese camino no existe: la vuelta
-- es por el esquema propio de la app (§7.1), y cada producto es su propio binario
-- con su propio esquema —los mismos que gogio declara al empaquetar—, así que
-- van los catorce, uno por app. Sin comodines ni prefijos: la validación de
-- redirect de este IdP compara la cadena entera, que es justo lo que impide que
-- otra app registre el esquema y se lleve el código.
--
-- UN client para los catorce y no catorce clients: es la MISMA suite y la misma
-- sesión; lo que cambia es por dónde vuelve el navegador del sistema. Su
-- audiencia entra en platform/pkg/config.firstPartyAudiences en este mismo
-- commit; sin eso, todos los servicios contestan 401 y se ve como pantallas
-- vacías (lo que le pasó a Sytue Fotos el 2026-09-10).
--
-- LO QUE ESTA MIGRACIÓN NO ARREGLA, y hay que decirlo: con el client dado de
-- alta, la app móvil TODAVÍA no tiene sesión. Faltan dos piezas del lado del
-- cliente, anotadas en docs/roadmap.md §Gio: abrir el navegador del sistema en
-- Android (un Intent por JNI) y guardar el refresh token en el almacén seguro
-- (Keystore / Keychain). Hasta entonces la sesión vive en memoria, que es peor
-- que un almacén seguro pero mejor que un token en claro.
--
-- La columna del identificador es `id`, NO `client_id` (ver 0057).
BEGIN;

INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes,
     redirect_uris, post_logout_redirect_uris)
VALUES
    ('sytue-mobile', 'Sytue Móvil', 'public', true, 'none',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        'sytue://auth/callback',
        'sytue-mail://auth/callback',
        'sytue-drive://auth/callback',
        'sytue-docs://auth/callback',
        'sytue-sheets://auth/callback',
        'sytue-slides://auth/callback',
        'sytue-pdf://auth/callback',
        'sytue-calendar://auth/callback',
        'sytue-contacts://auth/callback',
        'sytue-notes://auth/callback',
        'sytue-photos://auth/callback',
        'sytue-erp://auth/callback',
        'sytue-cloud://auth/callback',
        'sytue-admin://auth/callback'
     ],
     ARRAY[]::text[])
ON CONFLICT (id) DO NOTHING;

COMMIT;
