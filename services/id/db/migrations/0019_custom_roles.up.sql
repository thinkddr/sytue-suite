-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Roles personalizados / admin delegada (paridad Entra roles / Workspace admin roles):
-- hasta ahora la autorización de administración era binaria (owner/admin = TODO,
-- member = nada). Esto añade ROLES por organización que conceden un subconjunto de
-- PERMISOS GRANULARES (p. ej. «Help Desk» = users.manage pero NO billing.manage), sin
-- tocar owner/admin, que siguen siendo superusuarios built-in con acceso total.
--
-- MODELO. El catálogo de permisos es FIJO y vive en el código Go (domain/roles.go),
-- no en BD: son constantes con las que el servicio compila su enforcement (deny-by-
-- default). La BD solo guarda QUÉ permisos concede cada rol (permissions text[]) y a
-- QUIÉN se le asigna (user_custom_roles). Guardar el catálogo como texto (no como FK a
-- una tabla de permisos) evita una tabla de catálogo que solo el código conoce; el
-- handler valida cada permiso contra el catálogo Go antes de escribir.
--
-- ANTI AUTO-ESCALADA (crítico): `roles.manage` NO es delegable — definir/asignar roles
-- queda reservado a owner/admin (el handler exige owner/admin y RECHAZA roles.manage en
-- el conjunto de permisos de un rol personalizado). Así un rol delegado nunca puede
-- concederse a sí mismo la gestión de roles para escalar más allá de lo que tiene.
--
-- AISLAMIENTO POR TENANT (igual que mig 0001/0013/0018, a NIVEL DE APLICACIÓN): FK
-- org_id + scoping por org en CADA query, DENY-BY-DEFAULT. Un rol o una asignación de
-- la org A jamás se evalúan para la org B. Sin RLS de Postgres (el servicio no fija
-- GUCs por-conexión en el pool compartido).

CREATE TABLE custom_roles (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name        text NOT NULL,
    -- Permisos concedidos por el rol (constantes del catálogo Go, validadas por el
    -- handler). text[] y no una FK a una tabla de permisos: el catálogo es fijo y vive
    -- en el código, no hay nada que referenciar. '{}' = rol sin permisos (inerte).
    permissions text[] NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    -- Nombre único por-tenant (como grupos): dos orgs pueden tener «Help Desk».
    UNIQUE (org_id, name)
);

-- Listado admin por org, orden estable por (name, id) sin sort extra.
CREATE INDEX custom_roles_org_idx ON custom_roles (org_id, name, id);

CREATE TRIGGER custom_roles_set_updated_at BEFORE UPDATE ON custom_roles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Asignación de roles personalizados a usuarios. org_id denormalizado (= custom_roles.
-- org_id del rol) para acotar por tenant y servir la resolución de permisos efectivos
-- del usuario en CADA emisión de token con un solo índice (org_id, user_id), sin
-- depender del join. PK (role_id, user_id) evita duplicados. Todo cae en cascada al
-- borrar el rol, el usuario o la org.
CREATE TABLE user_custom_roles (
    org_id     uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    role_id    uuid NOT NULL REFERENCES custom_roles (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (role_id, user_id)
);

-- Permisos efectivos de un usuario en una org (unión de sus roles): se ejecuta en cada
-- emisión de token, como GroupsForUser. (org_id, user_id) lo sirve sin seq scan.
CREATE INDEX user_custom_roles_user_idx ON user_custom_roles (org_id, user_id);
