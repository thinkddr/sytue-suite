-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Contraseñas de aplicación: la credencial con la que un cliente de correo entra.
--
-- HOY NO EXISTE NINGUNA, y por eso Sytue Mail no se puede usar desde Thunderbird,
-- Outlook, Apple Mail ni el móvil. La suite publica su autoconfiguración —el
-- servicio sirve `/.well-known/autoconfig/mail/config-v1.1.xml` y Outlook
-- Autodiscover, con smtp.<dominio>:465 e imap.<dominio>:993—, o sea que invita a
-- configurar un cliente; pero el SMTP y el IMAP solo aceptan como contraseña un
-- JWT de Sytue ID con scope `mail` (services/mail/internal/idauth), que dura
-- minutos y que ninguna persona puede obtener ni escribir. Medido contra
-- producción el 2026-08-16: `535 5.7.8 Authentication failed` con la contraseña
-- de la cuenta.
--
-- Es la solución de siempre (Google, Proton, Fastmail): una credencial larga,
-- generada por el servidor, ligada a un uso concreto y revocable de una en una
-- sin tocar la contraseña de la persona ni su segundo factor.
--
-- POR QUÉ SHA-256 Y NO UN KDF LENTO, que es lo contrario de lo que pide una
-- contraseña de persona: esto NO lo elige nadie, lo genera el servidor con 20
-- bytes de aleatoriedad real. Contra eso un ataque de diccionario no existe, así
-- que el KDF solo aportaría latencia — y aquí sí duele: un cliente de correo
-- autentica en CADA conexión (IMAP abre varias a la vez y reconecta sin parar), y
-- un argon2 por conexión es un ataque de denegación servido de fábrica. Es el
-- mismo criterio con el que ya se guardan los tokens SCIM (mig. 0055).

CREATE TABLE app_passwords (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- SHA-256 (base64url) de la contraseña. El secreto NUNCA se guarda: se enseña
    -- una vez al crearlo y quien lo pierde crea otro. Único en toda la tabla
    -- porque es la clave por la que se busca al autenticar.
    secret_hash  text NOT NULL UNIQUE CHECK (secret_hash <> ''),
    -- Para qué es («Thunderbird del portátil», «iPhone»). Cuando hay tres, es lo
    -- único que dice cuál se está revocando.
    label        text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    -- Cuándo se usó por última vez. Es la mitad del valor de la pantalla: una
    -- contraseña creada hace un mes de la que no consta uso significa que ese
    -- cliente nunca llegó a conectarse, y hoy eso no se puede saber.
    last_used_at timestamptz,
    revoked_at   timestamptz
);

-- Las vivas de una persona: la consulta de la pantalla de seguridad.
CREATE INDEX app_passwords_user_idx ON app_passwords (user_id) WHERE revoked_at IS NULL;

COMMENT ON TABLE app_passwords IS
    'Contraseñas de aplicación para clientes de correo (IMAP/SMTP). Solo se guarda el hash; se enseñan una vez.';
