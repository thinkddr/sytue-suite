-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Comentarios por registro (bases-2, colaboración, tras 0005): un hilo de un nivel por registro.
-- Las mismas reglas que el resto de Bases (ver 0001 y 0004): RLS forzada con las dos
-- políticas, claves foráneas COMPUESTAS (tenant, id) —un comentario no puede colgar de
-- un registro ni de un hilo de otra organización aunque el código se equivocara— y
-- diferidas, para que la fusión de organizaciones pueda reescribir `tenant` tabla a tabla.
-- La respuesta lleva además el registro en su clave: no puede colgar de un hilo de OTRO registro.
CREATE TABLE base_comments (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant     text NOT NULL,
    base_id    uuid NOT NULL,
    table_id   uuid NOT NULL,
    record_id  uuid NOT NULL,
    -- NULL = abre hilo; con valor, responde a ese comentario raíz del MISMO registro.
    parent_id  uuid,
    author_sub text NOT NULL,
    body       text NOT NULL CHECK (length(body) <= 4000),
    created_at timestamptz NOT NULL DEFAULT now(),
    -- Borrado con respuestas vivas: queda la lápida (sin texto) para que el hilo no se
    -- quede huérfano; sin respuestas, la fila se borra de verdad.
    deleted_at timestamptz,
    CHECK ((deleted_at IS NULL) = (length(body) >= 1)),
    UNIQUE (tenant, id, record_id),
    FOREIGN KEY (tenant, table_id, base_id) REFERENCES base_tables (tenant, id, base_id)
        ON DELETE CASCADE ON UPDATE CASCADE DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (tenant, record_id, table_id) REFERENCES base_records (tenant, id, table_id)
        ON DELETE CASCADE ON UPDATE CASCADE DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (tenant, parent_id, record_id) REFERENCES base_comments (tenant, id, record_id)
        ON DELETE CASCADE ON UPDATE CASCADE DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX base_comments_por_registro ON base_comments (tenant, record_id, created_at, id);

ALTER TABLE base_comments ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_comments FORCE ROW LEVEL SECURITY;
CREATE POLICY base_comments_tenant_isolation ON base_comments
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_comments_platform_merge ON base_comments
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
