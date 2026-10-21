-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sytue Bases, núcleo: bases, permisos, tablas, campos tipados, registros,
-- enlaces entre registros, vistas guardadas y auditoría.
--
-- Reglas de TODA tabla con `tenant`:
--   · RLS ENABLE + FORCE con dos políticas: aislamiento (`app.tenant`) y fusión de
--     organizaciones (`app.merge`, la reconoce el preflight de Sytue ID).
--   · Las claves foráneas son COMPUESTAS (tenant, id): una comprobación de clave
--     foránea la hace el propietario y se salta la RLS, así que sin `tenant` en la
--     clave un id ajeno colaría un enlace entre organizaciones (IDOR). Con
--     ON UPDATE CASCADE para que la fusión (`UPDATE … SET tenant`) arrastre a los hijos.
--   · El borrado es seguro: un registro o una tabla a los que apunta un enlace NO se
--     borran (NO ACTION diferida, 23503 al confirmar → 409); un borrado en cascada de
--     la base entera sí, porque al confirmar ya no queda ningún enlace. Diferida y no
--     inmediata porque Postgres comprueba cada cascada por separado y vería el enlace
--     de una tabla que va a caer en la misma sentencia.

CREATE TABLE bases (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text NOT NULL,
    name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 1000),
    owner_sub   text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant, id)
);
CREATE INDEX bases_por_tenant ON bases (tenant, updated_at DESC);

CREATE TABLE base_permissions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant         text NOT NULL,
    base_id        uuid NOT NULL,
    -- NULL = toda la base; con valor, solo esa tabla (suma al rol de la base).
    table_id       uuid,
    principal_kind text NOT NULL CHECK (principal_kind IN ('user', 'tenant')),
    principal_id   text NOT NULL CHECK (length(principal_id) BETWEEN 1 AND 200),
    role           text NOT NULL CHECK (role IN ('editor', 'commenter', 'viewer')),
    created_by     text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant, base_id) REFERENCES bases (tenant, id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE UNIQUE INDEX base_permissions_unico ON base_permissions
    (base_id, coalesce(table_id, '00000000-0000-0000-0000-000000000000'::uuid), principal_kind, principal_id);
CREATE INDEX base_permissions_por_principal ON base_permissions (tenant, principal_kind, principal_id);

CREATE TABLE base_tables (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant     text NOT NULL,
    base_id    uuid NOT NULL,
    name       text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    position   integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant, id),
    UNIQUE (tenant, id, base_id),
    FOREIGN KEY (tenant, base_id) REFERENCES bases (tenant, id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX base_tables_por_base ON base_tables (tenant, base_id, position);
ALTER TABLE base_permissions ADD FOREIGN KEY (tenant, table_id, base_id)
    REFERENCES base_tables (tenant, id, base_id) ON DELETE CASCADE ON UPDATE CASCADE;

CREATE TABLE base_fields (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant        text NOT NULL,
    base_id       uuid NOT NULL,
    table_id      uuid NOT NULL,
    name          text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    kind          text NOT NULL CHECK (kind IN ('text', 'number', 'date', 'checkbox', 'select', 'multiselect', 'link')),
    -- select/multiselect: {"choices": ["…"]}. El tipo no cambia nunca tras crearse.
    options       jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (octet_length(options::text) <= 16384),
    -- link: la tabla de la MISMA base a la que apunta.
    link_table_id uuid,
    position      integer NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CHECK ((kind = 'link') = (link_table_id IS NOT NULL)),
    UNIQUE (tenant, id),
    UNIQUE (tenant, id, table_id),
    UNIQUE (tenant, id, link_table_id),
    FOREIGN KEY (tenant, table_id, base_id) REFERENCES base_tables (tenant, id, base_id) ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY (tenant, link_table_id, base_id) REFERENCES base_tables (tenant, id, base_id) ON UPDATE CASCADE
        DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX base_fields_por_tabla ON base_fields (tenant, table_id, position);

CREATE TABLE base_records (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant     text NOT NULL,
    base_id    uuid NOT NULL,
    table_id   uuid NOT NULL,
    -- {campo_id: valor} de los campos que no son enlaces.
    data       jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (octet_length(data::text) <= 65536),
    rev        bigint NOT NULL DEFAULT 1,
    created_by text NOT NULL,
    updated_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant, id),
    UNIQUE (tenant, id, table_id),
    FOREIGN KEY (tenant, table_id, base_id) REFERENCES base_tables (tenant, id, base_id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX base_records_por_tabla ON base_records (tenant, table_id, created_at, id);

CREATE TABLE base_links (
    tenant        text NOT NULL,
    field_id      uuid NOT NULL,
    from_record   uuid NOT NULL,
    from_table_id uuid NOT NULL,
    to_record     uuid NOT NULL,
    to_table_id   uuid NOT NULL,
    PRIMARY KEY (field_id, from_record, to_record),
    FOREIGN KEY (tenant, field_id, from_table_id) REFERENCES base_fields (tenant, id, table_id) ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY (tenant, field_id, to_table_id) REFERENCES base_fields (tenant, id, link_table_id) ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY (tenant, from_record, from_table_id) REFERENCES base_records (tenant, id, table_id) ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY (tenant, to_record, to_table_id) REFERENCES base_records (tenant, id, table_id) ON UPDATE CASCADE
        DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX base_links_hacia ON base_links (tenant, to_record);

CREATE TABLE base_views (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant     text NOT NULL,
    base_id    uuid NOT NULL,
    table_id   uuid NOT NULL,
    name       text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    kind       text NOT NULL DEFAULT 'grid' CHECK (kind IN ('grid', 'kanban', 'form')),
    -- {filters:[{field,op,value}], sort:[{field,dir}], group_by}. Validada en el servidor.
    config     jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (octet_length(config::text) <= 16384),
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant, id),
    FOREIGN KEY (tenant, table_id, base_id) REFERENCES base_tables (tenant, id, base_id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX base_views_por_tabla ON base_views (tenant, table_id, created_at);

-- La auditoría sobrevive al borrado de lo que audita: sin claves foráneas.
CREATE TABLE base_audit (
    id         bigserial PRIMARY KEY,
    tenant     text NOT NULL,
    base_id    uuid NOT NULL,
    table_id   uuid,
    record_id  uuid,
    field_id   uuid,
    actor      text NOT NULL,
    action     text NOT NULL CHECK (length(action) <= 64),
    -- Acotados a 1 KiB cada uno por el servidor (domain.Acotar).
    before     jsonb,
    after      jsonb,
    at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX base_audit_por_base ON base_audit (tenant, base_id, id DESC);

-- ---------------------------------------------------------------------------
-- Aislamiento por organización y fusión.
-- ---------------------------------------------------------------------------
ALTER TABLE bases            ENABLE ROW LEVEL SECURITY;
ALTER TABLE bases            FORCE ROW LEVEL SECURITY;
ALTER TABLE base_permissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_permissions FORCE ROW LEVEL SECURITY;
ALTER TABLE base_tables      ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_tables      FORCE ROW LEVEL SECURITY;
ALTER TABLE base_fields      ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_fields      FORCE ROW LEVEL SECURITY;
ALTER TABLE base_records     ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_records     FORCE ROW LEVEL SECURITY;
ALTER TABLE base_links       ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_links       FORCE ROW LEVEL SECURITY;
ALTER TABLE base_views       ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_views       FORCE ROW LEVEL SECURITY;
ALTER TABLE base_audit       ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_audit       FORCE ROW LEVEL SECURITY;

CREATE POLICY bases_tenant_isolation ON bases
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY bases_platform_merge ON bases
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY base_permissions_tenant_isolation ON base_permissions
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_permissions_platform_merge ON base_permissions
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY base_tables_tenant_isolation ON base_tables
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_tables_platform_merge ON base_tables
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY base_fields_tenant_isolation ON base_fields
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_fields_platform_merge ON base_fields
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY base_records_tenant_isolation ON base_records
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_records_platform_merge ON base_records
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY base_links_tenant_isolation ON base_links
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_links_platform_merge ON base_links
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY base_views_tenant_isolation ON base_views
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_views_platform_merge ON base_views
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY base_audit_tenant_isolation ON base_audit
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_audit_platform_merge ON base_audit
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
