-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Enlaces de un solo uso al perfil de Apple, para el iPhone.
--
-- iOS solo instala un perfil de configuración que se abre en Safari: una app no puede
-- dárselo al sistema. La app nativa pide un enlace con su sesión y lo abre en Safari,
-- que NO tiene esa sesión. El enlace es lo único que identifica a quien descarga, así
-- que vale UNA vez y unos minutos; al canjearlo se crea la contraseña de aplicación que
-- va dentro del perfil (mig 0056).
--
-- Solo se guarda el hash del token (SHA-256, como las contraseñas de aplicación): el
-- token es aleatorio, de 32 bytes, y lo genera el servidor.

CREATE TABLE apple_profile_links (
    token_hash text PRIMARY KEY CHECK (token_hash <> ''),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- La organización activa de quien lo pidió: de ella sale la autoridad S/MIME.
    tenant     text NOT NULL DEFAULT '',
    -- La etiqueta de la contraseña de aplicación que se creará («iPhone de Ada»).
    label      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at    timestamptz
);

-- Los caducados se barren por aquí.
CREATE INDEX apple_profile_links_expires_idx ON apple_profile_links (expires_at);

COMMENT ON TABLE apple_profile_links IS
    'Enlaces de un solo uso al perfil de Apple con su contraseña de aplicación (iOS los abre en Safari). Solo el hash del token.';
