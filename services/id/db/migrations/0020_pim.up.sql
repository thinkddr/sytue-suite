-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- PIM (Privileged Identity Management) — elevación de privilegios just-in-time
-- (paridad Entra PIM). Complementa los roles personalizados (mig 0019): en lugar de
-- ASIGNAR un rol de forma permanente (user_custom_roles ⇒ sus permisos SIEMPRE en el
-- JWT), un usuario puede ser ELEGIBLE para un rol y ACTIVARLO por una ventana acotada.
-- Solo mientras hay una activación ACTIVA (no expirada) los permisos del rol entran en
-- el claim `perms` del token; la mera elegibilidad NO concede nada (deny-by-default).
--
-- SEMÁNTICA perms ↔ TTL DEL TOKEN. Los perms se hornean en el access token en el
-- momento de emisión (buildTokens → EffectivePerms). Activar/expirar cambia el estado
-- en BD; el token lo refleja en la SIGUIENTE emisión (refresh). Por tanto una activación
-- se hace efectiva al re-mintar, y su caducidad se materializa como muy tarde al expirar
-- el access token vigente (TTL ~15 min). La expiración es PEREZOSA: se evalúa contra un
-- «ahora» en cada emisión (y la consulta del token filtra expires_at > now), sin barrido
-- de fondo — una fila 'active' pasada de expires_at deja de conceder aunque su status
-- siga en 'active'. ponytail: sin sweeper; añadir retención/purga si el histórico crece.
--
-- ROL: role_id (rol personalizado, FK) XOR builtin_role (owner/admin, texto). CHECK de
-- exclusión mutua. Un builtin activo concede el conjunto DELEGABLE de permisos (todos
-- menos roles.manage) sin tocar el claim `roles` (anti auto-escalada: los endpoints
-- reservados a owner/admin siguen gateados por requireRole, no por perms).
--
-- AISLAMIENTO POR TENANT (igual que 0019, a nivel de aplicación): FK org_id + scoping
-- por org en CADA query, deny-by-default. Sin RLS (el pool no fija GUCs por conexión).

CREATE TABLE role_eligibilities (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id               uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id              uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Exactamente uno: un rol personalizado de la org, o un rol built-in por nombre.
    role_id              uuid REFERENCES custom_roles (id) ON DELETE CASCADE,
    builtin_role         text,
    requires_approval    boolean NOT NULL DEFAULT false,
    -- Ventana MÁXIMA de una activación (minutos). El servicio acota la duración pedida.
    max_duration_minutes integer NOT NULL DEFAULT 60 CHECK (max_duration_minutes BETWEEN 1 AND 1440),
    created_at           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT role_eligibilities_role_xor CHECK ((role_id IS NULL) <> (builtin_role IS NULL))
);

-- Unicidad de la elegibilidad por (org, user, rol). role_id y builtin_role son
-- excluyentes ⇒ dos índices únicos parciales (los NULL no colisionarían de otro modo).
CREATE UNIQUE INDEX role_eligibilities_custom_uniq ON role_eligibilities (org_id, user_id, role_id)
    WHERE role_id IS NOT NULL;
CREATE UNIQUE INDEX role_eligibilities_builtin_uniq ON role_eligibilities (org_id, user_id, builtin_role)
    WHERE builtin_role IS NOT NULL;
-- Listado admin por org y resolución self por usuario (sirve ambos WHERE sin seq scan).
CREATE INDEX role_eligibilities_org_idx ON role_eligibilities (org_id, user_id);

CREATE TABLE role_activations (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id          uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id          uuid REFERENCES custom_roles (id) ON DELETE CASCADE,
    builtin_role     text,
    -- pending (espera aprobación) | active | revoked | denied | expired.
    status           text NOT NULL,
    justification    text NOT NULL DEFAULT '',
    -- Ventana pedida/concedida (minutos); expires_at = activated_at + duration.
    duration_minutes integer NOT NULL CHECK (duration_minutes BETWEEN 1 AND 1440),
    requested_at     timestamptz NOT NULL DEFAULT now(),
    activated_at     timestamptz,                             -- fijado al activar/aprobar
    expires_at       timestamptz,                             -- NULL mientras pending
    approved_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT role_activations_role_xor CHECK ((role_id IS NULL) <> (builtin_role IS NULL)),
    CONSTRAINT role_activations_status_chk CHECK (status IN ('pending', 'active', 'revoked', 'denied', 'expired'))
);

-- Fuente del claim `perms` en CADA emisión de token: activaciones ACTIVE y VIGENTES del
-- usuario en la org. (org_id, user_id, status) acota el barrido a las del usuario; el
-- filtro expires_at > now (en la consulta) lo cierra a las no caducadas. Sirve también
-- el listado self (todas las del usuario, sin el filtro de status).
CREATE INDEX role_activations_user_idx ON role_activations (org_id, user_id, status);
-- Vista admin de pendientes / vigentes por org.
CREATE INDEX role_activations_org_status_idx ON role_activations (org_id, status);
