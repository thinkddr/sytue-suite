-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Categorías de canal por usuario (Ola 11): grupos personalizados y colapsables del
-- sidebar, estilo Mattermost/Slack. Son estado PER-USUARIO (subject): cada usuario
-- agrupa y ordena su propia barra lateral sin afectar a los demás.
--
-- La asignación canal→categoría vive en la fila de pertenencia (channel_members), que
-- ya es la fila per-(usuario, canal): la columna category_id (NULL = categoría por
-- defecto según kind — Favoritos si favorite, DMs si direct, si no Canales) y sort_order
-- (orden dentro de la categoría). El sidebar lo lee en ListChannels vía el LEFT JOIN a
-- la propia membresía: una sola consulta, sin N+1.
--
-- Sin FK de channel_members.category_id → channel_categories: la PK compuesta
-- (tenant, subject, id) haría que un ON DELETE SET NULL anulara también tenant/subject.
-- La integridad la garantiza el store: DeleteCategory pone category_id=NULL en las filas
-- que la referenciaban (caen a la por defecto) dentro de la MISMA transacción del borrado.

CREATE TABLE channel_categories (
    tenant     text        NOT NULL,
    subject    text        NOT NULL,
    id         uuid        NOT NULL DEFAULT gen_random_uuid(),
    name       text        NOT NULL,
    sort_order integer     NOT NULL DEFAULT 0,
    collapsed  boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, subject, id),
    CONSTRAINT channel_categories_name_not_blank CHECK (length(name) > 0)
);

-- Orden de las categorías de un usuario: cubre el WHERE (tenant, subject) + ORDER BY
-- sort_order de ListCategories sin escanear las de otros usuarios. Escala.
CREATE INDEX channel_categories_order
    ON channel_categories (tenant, subject, sort_order);

-- Asignación canal→categoría (NULL = por defecto) + orden dentro de la categoría,
-- per-usuario, en la propia fila de pertenencia.
ALTER TABLE channel_members ADD COLUMN category_id uuid;
ALTER TABLE channel_members ADD COLUMN sort_order  integer NOT NULL DEFAULT 0;

-- Agrupar y ordenar los canales de una categoría de un usuario sin escanear toda su
-- membresía (el sidebar los pinta por categoría/orden). Escala a cientos de miles.
CREATE INDEX channel_members_category
    ON channel_members (tenant, subject, category_id, sort_order);

-- RLS deny-by-default por tenant (mismo patrón que el resto del esquema).
ALTER TABLE channel_categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE channel_categories FORCE ROW LEVEL SECURITY;
CREATE POLICY channel_categories_tenant_isolation ON channel_categories
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
