-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- CONSENTIMIENTO DE LAS APPS NATIVAS, UNA VEZ POR DISPOSITIVO (NAT-02; decidido por el
-- dueño el 2026-10-16: «Sí, una vez por dispositivo»).
--
-- Los clients nativos de la suite (`sytue-desktop`, 0069, y `sytue-mobile`, 0071) son
-- públicos: no tienen secreto propio. Sytue ID les entregaba el código sin preguntar, así
-- que con una sesión abierta en el navegador OTRA app del mismo móvil, u otra persona del
-- mismo equipo por el loopback, abría /authorize con su propio PKCE y se llevaba los
-- tokens. Ahora, la primera vez que una app nativa entra desde un dispositivo, Sytue ID
-- pregunta; después ya no.
--
-- QUÉ ES «EL DISPOSITIVO». Un secreto de 256 bits que la app crea la primera vez y guarda
-- en el almacén seguro del sistema (Keychain, Secret Service, Credential Manager, el
-- Keystore). A /authorize solo viaja su SHA-256 (`sytue_device`); el secreto viaja una
-- vez, en el canje del código por el canal de atrás (`sytue_device_secret`), como el
-- verificador de PKCE. El código queda atado a ese hash: quien copie el hash de otro
-- dispositivo sin tener su secreto obtiene un código que no puede canjear. Ni la
-- cookie del navegador ni el User-Agent sirven: los comparten todas las apps del
-- equipo, que es justo de quién hay que distinguir a la nuestra.
--
-- Se recuerda por cuenta, client, vuelta (redirect_uri: en el móvil cada app vuelve por
-- su esquema) y dispositivo. Sin columna `tenant`: el consentimiento es de la PERSONA, no
-- de una organización, así que no entra en la fusión (migcheck no pide política).

-- El hash del dispositivo con el que se pidió el código; '' = la app no lo mandó (una
-- versión anterior: pregunta cada vez y el código no queda atado).
ALTER TABLE auth_codes ADD COLUMN dispositivo text NOT NULL DEFAULT '';

CREATE TABLE consentimientos_nativos (
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    client_id    text        NOT NULL REFERENCES oauth_clients (id) ON DELETE CASCADE,
    redirect_uri text        NOT NULL,
    dispositivo  text        NOT NULL CHECK (length(dispositivo) = 43),
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, client_id, redirect_uri, dispositivo)
);

COMMENT ON TABLE consentimientos_nativos IS
    'Apps nativas a las que una persona dio acceso en un dispositivo (NAT-02): no se vuelve a preguntar.';

-- LOS TRES REDIRECTS HUÉRFANOS DE LA 0071. `sytue://`, `sytue-cloud://` y
-- `sytue-admin://` no los declara ninguna app (apps/fyne/cmd/*/FyneApp.toml; Admin y
-- DevX son solo web desde el 2026-10-01). Un esquema que no reclama ninguna app nuestra
-- lo puede registrar cualquier otra del teléfono y llevarse el código por él.
UPDATE oauth_clients
   SET redirect_uris = array_remove(array_remove(array_remove(redirect_uris,
           'sytue://auth/callback'), 'sytue-cloud://auth/callback'), 'sytue-admin://auth/callback')
 WHERE id = 'sytue-mobile';
