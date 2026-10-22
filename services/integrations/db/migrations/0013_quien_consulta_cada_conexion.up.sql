-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- QUIÉN PUEDE CONSULTAR CADA CONEXIÓN (NEG-06; decidido por el dueño el 2026-10-16:
-- «lista por conexión»).
--
-- Hasta ahora cualquier miembro de la organización consultaba un servicio conectado con
-- la credencial de la ORGANIZACIÓN: con Databricks, eso es cualquier SQL contra el
-- almacén de datos de la empresa. Desde aquí consultan los administradores, quien creó
-- la conexión y quien esté en su lista, una persona o un grupo de Sytue ID.
--
-- `created_by` es el `sub` de quien la creó. Las conexiones anteriores no lo guardaban y
-- se quedan con '': su lista nace vacía, así que solo las consultan los administradores
-- hasta que uno nombre a alguien. Es el defecto que decidió el dueño, aplicado también a
-- lo que ya existía, y les QUITA el acceso a los miembros que hoy consultaban.
ALTER TABLE app_instances ADD COLUMN created_by text NOT NULL DEFAULT '';

CREATE TABLE instance_readers (
    instance_id uuid        NOT NULL REFERENCES app_instances (id) ON DELETE CASCADE,
    org_id      text        NOT NULL,
    -- user: una persona, por su `sub`. group: un grupo de Sytue ID por su NOMBRE, que es
    -- lo que trae el claim `groups` del token (igual que la ACL de Bases).
    kind        text        NOT NULL CHECK (kind IN ('user', 'group')),
    principal   text        NOT NULL CHECK (length(btrim(principal)) BETWEEN 1 AND 255),
    -- Solo para enseñarlo en la consola: no decide nada.
    name        text        NOT NULL DEFAULT '' CHECK (length(name) <= 200),
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (instance_id, kind, principal)
);

-- «¿Qué conexiones puede consultar esta persona?» (la búsqueda federada) va por persona
-- y por sus grupos, no por conexión.
CREATE INDEX instance_readers_principal ON instance_readers (org_id, kind, principal);

ALTER TABLE instance_readers ENABLE ROW LEVEL SECURITY;
ALTER TABLE instance_readers FORCE ROW LEVEL SECURITY;

CREATE POLICY instance_readers_org_isolation ON instance_readers
    USING (org_id = current_setting('app.tenant', true))
    WITH CHECK (org_id = current_setting('app.tenant', true));

GRANT SELECT, INSERT, UPDATE, DELETE ON instance_readers TO integrations_app;
