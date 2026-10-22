-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Almacenamiento de TERCEROS por tenant (ADR 0038, 2026-09-13): todos los planes
-- permiten conectar un bucket S3 propio. Conectado, los blobs NUEVOS del tenant
-- van a ese bucket (clave con prefijo `ext/`, ver service.blobRouter) y no
-- cuentan en la cuota de Sytue; los de antes siguen donde estaban, que es lo
-- que hace que desconectarlo no rompa nada ya escrito.
--
-- La clave secreta se guarda SELLADA con la KEK del servicio
-- (EXTERNAL_STORAGE_KEY, keycrypto): sin clave, la función queda apagada.

CREATE TABLE external_storage (
    tenant     text PRIMARY KEY,
    -- Clase de almacén: un bucket S3 o un servidor WebDAV (NAS, Nextcloud, la
    -- CLI de Internxt…). Cada clase lee endpoint/credenciales a su manera.
    kind       text NOT NULL DEFAULT 's3' CHECK (kind IN ('s3', 'webdav', 'gdrive', 'onedrive', 'nas')),
    -- Clase `nas`: un WebDAV en las instalaciones del cliente, publicado hacia
    -- la suite por el túnel de Sytue Cloud. Drive crea el sitio y el recurso en
    -- el proxy de la plataforma y recuerda sus ids para desmontarlos; la
    -- credencial del agente va sellada para poder volver a enseñar el comando.
    tunnel_site_id     integer NOT NULL DEFAULT 0,
    tunnel_resource_id integer NOT NULL DEFAULT 0,
    tunnel_newt_id     text    NOT NULL DEFAULT '',
    tunnel_newt_secret_enc text NOT NULL DEFAULT '',
    -- La dirección LOCAL del NAS (http://192.168.1.50:5005), que es a donde el
    -- agente reenvía; `endpoint` guarda la pública del túnel.
    local_endpoint text NOT NULL DEFAULT '',
    -- Google Drive y OneDrive van por consentimiento OAuth: la instancia del
    -- servicio de integraciones que guarda el token. Las otras clases no la usan.
    instance_id text NOT NULL DEFAULT '',
    -- host:puerto del S3 del tenant (el esquema fija use_ssl), o la URL base del
    -- WebDAV. base_path es la carpeta dentro del almacén (WebDAV) o el prefijo (S3).
    endpoint   text NOT NULL,
    base_path  text NOT NULL DEFAULT '',
    region     text NOT NULL DEFAULT '',
    bucket     text NOT NULL,
    use_ssl    boolean NOT NULL DEFAULT true,
    access_key text NOT NULL,
    -- Sellado con keycrypto; nunca se devuelve por la API. Vacío en las clases
    -- que van por OAuth (el token vive en el servicio de integraciones).
    secret_key_enc text NOT NULL DEFAULT '',
    -- Nombre para reconocerlo («el Wasabi de la oficina»). Decoración.
    label      text NOT NULL DEFAULT '',
    -- Apagarlo sin perder la configuración: lo escrito allí se sigue leyendo.
    enabled    boolean NOT NULL DEFAULT true,
    -- Última comprobación (BucketExists con las credenciales dadas).
    checked_at  timestamptz,
    check_ok    boolean NOT NULL DEFAULT false,
    check_error text    NOT NULL DEFAULT '',
    -- Cuándo aceptó quien lo conectó el aviso de responsabilidad: las copias de
    -- seguridad y la recuperación de lo que vive en un almacén conectado son del
    -- cliente, y Sytue no responde de su pérdida. Sin aceptarlo no se guarda.
    accepted_terms_at timestamptz NOT NULL,
    updated_by text        NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT external_storage_endpoint_not_blank CHECK (length(endpoint) > 0),
    CONSTRAINT external_storage_bucket_not_blank   CHECK (length(bucket) > 0)
);

ALTER TABLE external_storage ENABLE ROW LEVEL SECURITY;
ALTER TABLE external_storage FORCE ROW LEVEL SECURITY;
CREATE POLICY external_storage_isolation ON external_storage
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY external_storage_maintenance ON external_storage
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
CREATE POLICY external_storage_merge ON external_storage
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

-- Google Drive identifica cada fichero por un id suyo, no por un nombre: este
-- mapa guarda, por clave de blob, el id que dio el proveedor. OneDrive y el
-- resto van por ruta y no lo necesitan.
CREATE TABLE external_objects (
    tenant      text NOT NULL,
    storage_key text NOT NULL,
    provider_id text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, storage_key)
);
ALTER TABLE external_objects ENABLE ROW LEVEL SECURITY;
ALTER TABLE external_objects FORCE ROW LEVEL SECURITY;
CREATE POLICY external_objects_isolation ON external_objects
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY external_objects_maintenance ON external_objects
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
CREATE POLICY external_objects_merge ON external_objects
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
