-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- ADJUNTOS (ADR 0068, bases-2, pieza 4): un campo de tipo `attachment` guarda ficheros.
--
-- Los bytes van al almacén de objetos (platform/pkg/objectstore) y aquí queda la ficha:
-- nombre, tipo —el que dice el CONTENIDO, no quien sube—, tamaño y huella. La clave del
-- objeto no la elige nadie: es `bases/<base>/<adjunto>` bajo el prefijo de la
-- organización (`store_tenant`, que NO cambia en una fusión de organizaciones: el objeto
-- se queda donde se subió).
--
-- Borrado seguro: la ficha cae en cascada con su registro, su campo, su tabla o su base,
-- y un trigger anota la clave en `base_attachment_purge`, de donde el servicio borra el
-- objeto. Así ninguna vía de borrado tiene que acordarse del almacén.
ALTER TABLE base_fields DROP CONSTRAINT base_fields_kind_check;
ALTER TABLE base_fields ADD CONSTRAINT base_fields_kind_check
    CHECK (kind IN ('text', 'number', 'date', 'checkbox', 'select', 'multiselect', 'link', 'formula', 'attachment'));

CREATE TABLE base_attachments (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant       text NOT NULL,
    store_tenant text NOT NULL,
    base_id      uuid NOT NULL,
    table_id     uuid NOT NULL,
    record_id    uuid NOT NULL,
    field_id     uuid NOT NULL,
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 255),
    mime         text NOT NULL CHECK (length(mime) BETWEEN 1 AND 100),
    size         bigint NOT NULL CHECK (size > 0),
    sha256       text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    has_thumb    boolean NOT NULL DEFAULT false,
    created_by   text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant, id),
    FOREIGN KEY (tenant, record_id, table_id) REFERENCES base_records (tenant, id, table_id)
        ON DELETE CASCADE ON UPDATE CASCADE DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (tenant, field_id, table_id) REFERENCES base_fields (tenant, id, table_id)
        ON DELETE CASCADE ON UPDATE CASCADE DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (tenant, table_id, base_id) REFERENCES base_tables (tenant, id, base_id)
        ON DELETE CASCADE ON UPDATE CASCADE DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX base_attachments_por_registro ON base_attachments (tenant, record_id, field_id, created_at, id);
-- La cuota es por base: lo que suman sus adjuntos.
CREATE INDEX base_attachments_por_base ON base_attachments (tenant, base_id);

ALTER TABLE base_attachments ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_attachments FORCE ROW LEVEL SECURITY;
CREATE POLICY base_attachments_tenant_isolation ON base_attachments
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_attachments_platform_merge ON base_attachments
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

-- Objetos por borrar del almacén. Sin claves foráneas: sobrevive a lo que se borró.
-- `after` deja margen a una subida en curso: se anota ANTES de subir, y si la subida no
-- llega a confirmarse el objeto se borra pasado ese plazo.
CREATE TABLE base_attachment_purge (
    id           bigserial PRIMARY KEY,
    tenant       text NOT NULL,
    store_tenant text NOT NULL,
    object_key   text NOT NULL CHECK (object_key ~ '^bases/[0-9a-f-]{36}/[0-9a-f-]{36}$'),
    after        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX base_attachment_purge_por_fecha ON base_attachment_purge (after);

ALTER TABLE base_attachment_purge ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_attachment_purge FORCE ROW LEVEL SECURITY;
CREATE POLICY base_attachment_purge_tenant_isolation ON base_attachment_purge
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_attachment_purge_platform_merge ON base_attachment_purge
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
-- Solo lectura, para que el bucle de purga encuentre trabajo sin saber de qué
-- organización es; borrar la fila va en la transacción de ESA organización.
CREATE POLICY base_attachment_purge_publico ON base_attachment_purge FOR SELECT
    USING (current_setting('app.bases_publico', true) = 'on');

CREATE FUNCTION base_attachments_purgar() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO base_attachment_purge (tenant, store_tenant, object_key)
    SELECT tenant, store_tenant, 'bases/' || base_id || '/' || id FROM borradas;
    RETURN NULL;
END $$;
CREATE TRIGGER base_attachments_purgar AFTER DELETE ON base_attachments
    REFERENCING OLD TABLE AS borradas FOR EACH STATEMENT EXECUTE FUNCTION base_attachments_purgar();
