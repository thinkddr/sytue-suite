-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Dispositivos de confianza (paridad Entra device-based Conditional Access): al
-- primer login desde un navegador, Sytue ID emite una COOKIE DE DISPOSITIVO firmada
-- (HMAC con la clave del servicio, httpOnly, long-lived) y registra el dispositivo.
-- En logins siguientes la cookie lo identifica. `trusted` por defecto FALSE: el
-- usuario (o un admin) lo marca de confianza explícitamente. El Acceso Condicional
-- (mig 0018) gana una condición `require_trusted_device`: si la política la exige y
-- el login NO viene de un dispositivo de confianza del usuario, aplica su acción
-- (block / require_mfa). El enforcement vive en el login del servicio, no en el cliente.
--
-- IDENTIDAD DE DISPOSITIVO: la cookie guarda un SECRETO DE NAVEGADOR opaco `B`; el
-- device_id de esta tabla se DERIVA por HMAC(clave, user_id || B) — de ahí que sea
-- ÚNICO GLOBALMENTE y a la vez ESTABLE por (usuario, navegador): dos usuarios que
-- comparten un navegador obtienen dispositivos distintos (la confianza es POR USUARIO,
-- como en Entra), sin colisión ni filas duplicadas al alternar cuentas. La cookie
-- nunca contiene el device_id (es un HMAC no invertible del secreto).
--
-- AISLAMIENTO POR TENANT (igual que mig 0001/0018, a NIVEL DE APLICACIÓN): FK org_id
-- + scoping por org/usuario en CADA query, deny-by-default. Un dispositivo de otra org
-- jamás se lista ni se revoca fuera de ella.

CREATE TABLE devices (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Identificador estable del dispositivo derivado del secreto de la cookie y del
    -- usuario (HMAC): opaco, único globalmente. NO es el valor de la cookie.
    device_id   text NOT NULL UNIQUE,
    -- Nombre legible derivado del User-Agent en el primer registro ("Chrome · macOS").
    name        text NOT NULL DEFAULT '',
    user_agent  text NOT NULL DEFAULT '',
    -- Marca de confianza. FALSE por defecto: solo se eleva por acción explícita del
    -- usuario (sobre su dispositivo actual) o de un administrador.
    trusted     boolean NOT NULL DEFAULT false,
    first_seen  timestamptz NOT NULL DEFAULT now(),
    last_seen   timestamptz NOT NULL DEFAULT now(),
    last_ip     text NOT NULL DEFAULT ''
);

-- Listado self-service (mis dispositivos, recientes primero) y admin por org.
CREATE INDEX devices_user_idx ON devices (user_id, last_seen DESC);
CREATE INDEX devices_org_idx  ON devices (org_id, last_seen DESC);

-- Acceso Condicional (mig 0018): condición "requiere dispositivo de confianza". Si la
-- política la exige y el login NO viene de un dispositivo `trusted` del usuario, la
-- política aplica su acción. FALSE por defecto ⇒ no cambia ninguna política existente.
ALTER TABLE access_policies
    ADD COLUMN require_trusted_device boolean NOT NULL DEFAULT false;
