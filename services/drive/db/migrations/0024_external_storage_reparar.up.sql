-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Repara las bases que aplicaron la 0023 ANTES de que se reescribiera.
--
-- La 0023 salió en la v2.81.0 solo con S3. En la v2.84.0 se EDITÓ para añadir
-- las clases WebDAV, Google Drive, OneDrive y NAS (4d8719ab5) en lugar de añadir
-- una migración nueva, y donde ya estaba aplicada —producción— nada de eso llegó
-- a crearse: GET /api/v1/storage/external respondía 500 desde entonces. Medido el
-- 2026-09-15 en los logs de drive v2.87.0: «column "kind" does not exist», y la
-- pantalla de almacenamiento conectado de Drive sin poder cargarse.
--
-- Todo es idempotente: en una base nueva, que ya trae la 0023 completa, no cambia
-- nada. Una migración aplicada no se reescribe; se añade otra.

ALTER TABLE external_storage
    ADD COLUMN IF NOT EXISTS kind text NOT NULL DEFAULT 's3'
        CHECK (kind IN ('s3', 'webdav', 'gdrive', 'onedrive', 'nas')),
    ADD COLUMN IF NOT EXISTS tunnel_site_id integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS tunnel_resource_id integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS tunnel_newt_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS tunnel_newt_secret_enc text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS local_endpoint text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS instance_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS base_path text NOT NULL DEFAULT '',
    -- Una conexión guardada antes del aviso de responsabilidad NO lo aceptó: se
    -- marca con 'epoch' («no consta») en vez de inventarle una fecha, y el próximo
    -- guardado exige aceptarlo. El DEFAULT solo sirve para rellenar esas filas;
    -- se retira abajo para quedar como en la 0023.
    ADD COLUMN IF NOT EXISTS accepted_terms_at timestamptz NOT NULL DEFAULT 'epoch',
    ALTER COLUMN secret_key_enc SET DEFAULT '';
ALTER TABLE external_storage ALTER COLUMN accepted_terms_at DROP DEFAULT;

CREATE TABLE IF NOT EXISTS external_objects (
    tenant      text NOT NULL,
    storage_key text NOT NULL,
    provider_id text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, storage_key)
);
ALTER TABLE external_objects ENABLE ROW LEVEL SECURITY;
ALTER TABLE external_objects FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS external_objects_isolation ON external_objects;
CREATE POLICY external_objects_isolation ON external_objects
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
DROP POLICY IF EXISTS external_objects_maintenance ON external_objects;
CREATE POLICY external_objects_maintenance ON external_objects
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
DROP POLICY IF EXISTS external_objects_merge ON external_objects;
CREATE POLICY external_objects_merge ON external_objects
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
