-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- SYTUE GIT COMO CLIENTE OIDC. La forja tenía cuentas y contraseñas propias, que
-- es exactamente lo que Sytue ID existe para evitar: una credencial más que
-- rotar, un sitio más del que dar de baja a quien se va, y un segundo factor que
-- configurar aparte. Con esto, entrar en la forja es entrar en la suite.
--
-- CALLBACK: Gitea construye la ruta de retorno como
-- /user/oauth2/<nombre-de-la-fuente>/callback. La fuente se llama «sytue-id»
-- (ver deploy/sytue-git/bin/configurar-oidc.sh), así que ese nombre forma parte
-- del contrato: cambiarlo en la forja obliga a cambiarlo aquí.
--
-- SIN offline_access: la forja mantiene su propia sesión tras el intercambio y
-- no necesita refrescar tokens contra el IdP. Pedir un refresh token que nadie
-- va a usar solo alarga la vida de una credencial de más.
--
-- SIN PKCE, Y ESTO ES UNA CONCESIÓN CONSCIENTE. Gitea delega el flujo OIDC en
-- markbates/goth, que no implementa PKCE para openidConnect: el verificador
-- tendría que sobrevivir entre la ida al IdP y la vuelta con el código, y ahí no
-- hay dónde guardarlo. Implementarlo obligaría a tocar el flujo de sesión de la
-- forja y a mantener esa divergencia en cada merge con upstream.
--
-- Lo que sostiene la seguridad sin PKCE, y que NO se puede relajar:
--   · es un cliente CONFIDENCIAL: canjear el código exige el secreto,
--   · los redirect_uris son EXACTOS y están enumerados aquí —sin comodines—,
--     así que un código robado no se puede canjear hacia otro sitio,
--   · el `state` lo valida la forja en la vuelta.
-- Un cliente PÚBLICO en estas condiciones sí sería vulnerable a interceptación
-- del código; uno confidencial con retorno exacto, no.
--
-- Si algún día goth soporta PKCE, esto vuelve a true y se quita este párrafo.
--
-- El secret_hash queda NULL: se fija al desplegar desde el Secret Manager, como
-- el resto de clientes confidenciales (ver 0002). Con NULL, el endpoint de token
-- rechaza la autenticación, así que un despliegue a medias falla en vez de
-- quedarse abierto.
BEGIN;

INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes,
     redirect_uris, post_logout_redirect_uris)
VALUES
    ('sytue-git', 'Sytue Git', 'confidential', false, 'client_secret_basic',
     '{authorization_code}', '{code}', '{openid,profile,email}',
     ARRAY[
        'https://git.sytue.eu/user/oauth2/sytue-id/callback',
        'https://git.sytue.localhost/user/oauth2/sytue-id/callback',
        'http://localhost:3000/user/oauth2/sytue-id/callback',
        -- 127.0.0.1 además de localhost: en desarrollo la forja escucha en IPv4 y
        -- «localhost» resuelve antes a ::1, así que el navegador puede acabar en
        -- cualquiera de las dos y el retorno tiene que valer para ambas.
        'http://127.0.0.1:3000/user/oauth2/sytue-id/callback'
     ],
     ARRAY['https://git.sytue.eu/', 'http://localhost:3000/', 'http://127.0.0.1:3000/'])
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    require_pkce = EXCLUDED.require_pkce,
    redirect_uris = EXCLUDED.redirect_uris,
    post_logout_redirect_uris = EXCLUDED.post_logout_redirect_uris,
    scopes = EXCLUDED.scopes;

COMMIT;
