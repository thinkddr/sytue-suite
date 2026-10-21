-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El token de aprovisionamiento SCIM, POR ORGANIZACIÓN y creado desde la consola.
--
-- Hasta ahora el token era UNO para todo el despliegue (`SCIM_BEARER_TOKEN`) y
-- estaba ligado a UNA organización por otra variable (`SCIM_ORG_SLUG`). Eso
-- tiene dos consecuencias que lo hacían inservible como producto:
--
--   · Para las demás organizaciones SCIM no existe. No es que esté apagado: es
--     que no hay forma de encenderlo sin editar el despliegue de la plataforma
--     entera y reiniciar el IdP.
--   · Nadie puede configurarlo por su cuenta. Un cliente que quiere conectar su
--     Entra ID tiene que pedirle a Sytue que le ponga una variable de entorno,
--     y eso no es una función, es un ticket.
--
-- De ahí esta tabla: cada organización crea el suyo desde su consola, se le
-- enseña UNA vez y se guarda solo el hash. Rotarlo es crear otro y revocar el
-- viejo; revocarlo corta el aprovisionamiento en el acto, sin desplegar nada.
--
-- El token del entorno SIGUE VALIENDO mientras exista (compatibilidad con el
-- despliegue actual): el middleware lo comprueba primero y, si no casa, busca
-- aquí. Quitarlo del chart es una decisión aparte y no puede ser un efecto
-- secundario de esta migración.

CREATE TABLE scim_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    -- SHA-256 en hexadecimal del token. El secreto NUNCA se guarda: quien lo
    -- pierde crea otro. Único en toda la tabla porque es la clave por la que se
    -- busca al autenticar, y dos organizaciones no pueden compartir token.
    token_hash   text NOT NULL UNIQUE CHECK (token_hash <> ''),
    -- Etiqueta libre para distinguirlos («Entra ID», «Okta pruebas»). No es
    -- decorativa: cuando hay dos, es lo único que dice cuál se está revocando.
    label        text NOT NULL DEFAULT '',
    created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    -- Cuándo se usó por última vez. Es la mitad del valor de la pantalla: un
    -- token creado hace meses del que no consta uso significa que la conexión
    -- nunca llegó a funcionar, y eso hoy no se puede saber.
    last_used_at timestamptz,
    revoked_at   timestamptz
);

-- Los vivos de una organización, que es la consulta de la pantalla.
CREATE INDEX scim_tokens_org_idx ON scim_tokens (org_id) WHERE revoked_at IS NULL;

COMMENT ON TABLE scim_tokens IS
    'Tokens de aprovisionamiento SCIM por organización, creados desde la consola. Solo se guarda el hash.';
