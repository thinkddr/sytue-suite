-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Esquema base de Sytue Photos: el INDICE de la galeria y las recetas de edicion.
--
-- LO QUE NO ESTA AQUI, que es la decision que ordena el resto: los BYTES. El
-- original vive en Sytue Drive (`drive_file_id`) y las derivadas en el almacen
-- de objetos, direccionadas por (activo, huella de receta, variante). Esta base
-- de datos solo sabe DONDE estan y como se veian: fecha, tamano y proporcion,
-- que es lo justo para pintar la linea de tiempo sin abrir un solo fichero.
--
-- Una galeria que tiene que leer el original para saber donde colocar un hueco
-- no puede desplazarse con fluidez, y por eso `width`/`height` son NOT NULL:
-- ingerir sin medir dejaria la cuadricula saltando al cargar cada foto.
--
-- Clean-room: modelo propio. La inspiracion funcional -linea de tiempo por fecha
-- de captura, albumes, edicion no destructiva por receta- son ideas y no tienen
-- copyright; no se ha consultado ni portado codigo de terceros (CONVENTIONS §12).
--
-- Multi-tenant: cada fila lleva `tenant` (slug). RLS deny-by-default aisla por la
-- variable de sesion `app.tenant` (la fija pg.SetTenant en cada transaccion). La
-- autorizacion POR USUARIO la aplica el servicio ademas de RLS: RLS separa
-- organizaciones, no personas.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Activos: una foto o un video indexado.
-- ---------------------------------------------------------------------------
CREATE TABLE assets (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    owner_sub       text        NOT NULL,

    -- El original, en Drive. UNICO por organizacion: reingerir el mismo fichero
    -- no puede crear una segunda entrada, o la linea de tiempo saldria doble.
    drive_file_id   text        NOT NULL,
    name            text        NOT NULL,
    media_type      text        NOT NULL,
    bytes           bigint      NOT NULL CHECK (bytes >= 0),

    -- Dimensiones DEL SENSOR, sin aplicar la orientacion: es lo que dice el
    -- fichero. Quien pinta consulta `orientation` y las intercambia si toca.
    width           integer     NOT NULL CHECK (width  > 0),
    height          integer     NOT NULL CHECK (height > 0),
    -- Etiqueta EXIF 0x0112 (1..8). Se guarda TAL CUAL, incluido un valor raro:
    -- el dominio lo trata como "sin rotacion" y avisa, en vez de perder la foto
    -- por una etiqueta mal escrita por la camara.
    orientation     smallint    NOT NULL DEFAULT 1,

    -- Fecha de captura del EXIF. NULA cuando el fichero no la trae, y entonces
    -- la galeria cae a `uploaded_at` y LO DICE: colocar una descarga de 2019
    -- entre las fotos de hoy sin avisar es peor que no ordenarla.
    taken_at        timestamptz,
    uploaded_at     timestamptz NOT NULL DEFAULT now(),

    -- Camara y objetivo, para filtrar. Texto libre porque el EXIF lo es.
    camera          text        NOT NULL DEFAULT '',
    lens            text        NOT NULL DEFAULT '',

    -- Papelera propia, separada de la de Drive: quitar una foto de la galeria
    -- no puede borrar el fichero de quien lo tenia en una carpeta compartida.
    deleted_at      timestamptz,

    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT assets_drive_unico UNIQUE (tenant, drive_file_id)
);

-- El indice de la LINEA DE TIEMPO, que es la consulta que se hace siempre:
-- las fotos de una persona, mas recientes primero, sin las de la papelera.
--
-- `coalesce(taken_at, uploaded_at)` DENTRO del indice y no en la consulta: sin
-- esto Postgres no puede usarlo para ordenar y acaba ordenando en memoria toda
-- la coleccion en cada scroll.
CREATE INDEX assets_timeline ON assets (tenant, owner_sub, coalesce(taken_at, uploaded_at) DESC, id DESC)
    WHERE deleted_at IS NULL;

-- Vaciar la papelera y contar lo que ocupa.
CREATE INDEX assets_papelera ON assets (tenant, deleted_at) WHERE deleted_at IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Ediciones: la receta vigente de cada activo.
--
-- UNA POR ACTIVO Y PERSONA, no un historial de versiones. El historial ya lo da
-- la propia receta -deshacer es quitar un paso- y guardar cada estado
-- intermedio multiplicaria las filas por cada arrastre de un deslizador.
--
-- `fingerprint` se guarda AUNQUE se pueda recalcular de `recipe`: es lo que
-- nombra las derivadas, asi que tenerlo en columna permite localizar los
-- ficheros de una edicion sin deserializar y rehashear cada fila.
-- ---------------------------------------------------------------------------
CREATE TABLE edits (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    asset_id        uuid        NOT NULL REFERENCES assets (id) ON DELETE CASCADE,
    owner_sub       text        NOT NULL,

    recipe          jsonb       NOT NULL,
    fingerprint     text        NOT NULL,

    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT edits_una_por_persona UNIQUE (tenant, asset_id, owner_sub)
);

-- ---------------------------------------------------------------------------
-- Albumes: agrupacion manual. La linea de tiempo es automatica por fecha; esto
-- es lo que la persona decide.
-- ---------------------------------------------------------------------------
CREATE TABLE albums (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    owner_sub       text        NOT NULL,
    title           text        NOT NULL,
    -- La portada es un activo, y se pone a NULL si esa foto se va: sin el
    -- ON DELETE SET NULL, borrar una foto bloquearia el borrado del album.
    cover_asset_id  uuid        REFERENCES assets (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX albums_de_la_persona ON albums (tenant, owner_sub, updated_at DESC);

-- Pertenencia. `position` permite ordenar a mano dentro del album, que es
-- justo lo que distingue un album de un filtro por fecha.
CREATE TABLE album_assets (
    tenant          text        NOT NULL,
    album_id        uuid        NOT NULL REFERENCES albums (id) ON DELETE CASCADE,
    asset_id        uuid        NOT NULL REFERENCES assets (id) ON DELETE CASCADE,
    position        integer     NOT NULL DEFAULT 0,
    added_at        timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (album_id, asset_id)
);

CREATE INDEX album_assets_orden ON album_assets (tenant, album_id, position, added_at);

-- ---------------------------------------------------------------------------
-- Aislamiento por organizacion.
-- ---------------------------------------------------------------------------
ALTER TABLE assets       ENABLE ROW LEVEL SECURITY;
ALTER TABLE edits        ENABLE ROW LEVEL SECURITY;
ALTER TABLE albums       ENABLE ROW LEVEL SECURITY;
ALTER TABLE album_assets ENABLE ROW LEVEL SECURITY;

-- FORCE ademas de ENABLE: sin el, el dueno de la tabla se salta la politica, y
-- el usuario de la aplicacion suele serlo.
ALTER TABLE assets       FORCE ROW LEVEL SECURITY;
ALTER TABLE edits        FORCE ROW LEVEL SECURITY;
ALTER TABLE albums       FORCE ROW LEVEL SECURITY;
ALTER TABLE album_assets FORCE ROW LEVEL SECURITY;

CREATE POLICY assets_tenant_isolation ON assets
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY edits_tenant_isolation ON edits
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY albums_tenant_isolation ON albums
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY album_assets_tenant_isolation ON album_assets
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
