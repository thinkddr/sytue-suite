-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Esquema base de Sytue Notepads: cuadernos anidados (el arbol de la barra lateral),
-- notas libres dentro de cada cuaderno (lienzo libre, columnas o lista),
-- etiquetas con estado, y enlaces a objetos de la suite (eventos de Calendar,
-- salas y mensajes de Rooms, documentos de Docs, ficheros de Drive).
--
-- Clean-room: modelo propio. La INSPIRACION funcional (contenedores anidados de
-- notas heterogeneas, etiquetas con estado, filtro por etiqueta) son ideas, que
-- no tienen copyright; no se ha consultado ni portado codigo de terceros.
--
-- Multi-tenant: cada fila lleva `tenant` (slug). RLS deny-by-default aisla por la
-- variable de sesion `app.tenant` (la fija pg.SetTenant en cada transaccion). La
-- autorizacion POR USUARIO (visibilidad privada/personal/organizacion/publica) la
-- aplica el servicio ademas de RLS: RLS separa organizaciones, no personas.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Cuadernos: el arbol de contenedores. parent_id nulo = raiz.
--
-- visibility e indexable viven AQUI porque es donde el usuario los entiende:
-- se decide una vez para "Personal" o "Equipo" y todo lo que caiga dentro lo
-- hereda. Cada nota puede sobreescribirlos (columnas homonimas NULLABLE en
-- `notes`), que es lo que permite una nota privada dentro de un cuaderno
-- compartido sin obligar a mover nada de sitio.
-- ---------------------------------------------------------------------------
CREATE TABLE notebooks (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    owner_sub       text        NOT NULL,
    parent_id       uuid        REFERENCES notebooks (id) ON DELETE CASCADE,
    title           text        NOT NULL,
    -- Icono del cuaderno: un emoji. Es el selector de icono que todo el mundo
    -- ya sabe usar y no necesita una biblioteca de glifos ni un subidor.
    emoji           text        NOT NULL DEFAULT '',
    color           text        NOT NULL DEFAULT '#86198f',
    -- Disposicion del lienzo: libre (mapa mental), columnas o lista.
    layout          text        NOT NULL DEFAULT 'columns'
                        CHECK (layout IN ('free', 'columns', 'list')),
    -- Quien puede verlo: solo yo y sin indexar / solo yo / toda la organizacion /
    -- cualquiera con el enlace publico.
    visibility      text        NOT NULL DEFAULT 'personal'
                        CHECK (visibility IN ('private', 'personal', 'organization', 'public')),
    -- Entra en la busqueda unificada del Hub. Un cuaderno cifrado NUNCA se indexa
    -- (el servicio lo fuerza a false; aqui queda la constraint que lo garantiza).
    indexable       boolean     NOT NULL DEFAULT true,
    -- Cifrado extremo a extremo: el navegador cifra titulo y contenido antes de
    -- enviarlos. El servidor guarda el criptograma y los PARAMETROS de derivacion,
    -- nunca la contrasena ni la clave.
    encrypted       boolean     NOT NULL DEFAULT false,
    crypto          jsonb       NOT NULL DEFAULT '{}'::jsonb,
    -- Sala de Sytue Rooms vinculada: el cuaderno es el lienzo compartido de esa
    -- sala. Vacio = cuaderno suelto.
    room_id         text        NOT NULL DEFAULT '',
    -- Token del enlace publico (solo con visibility='public'); vacio si no.
    public_token    text        NOT NULL DEFAULT '',
    position        integer     NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz,
    CONSTRAINT notebook_title_not_blank CHECK (length(title) > 0),
    CONSTRAINT notebook_title_len CHECK (length(title) <= 200),
    CONSTRAINT notebook_color_format CHECK (color ~ '^#[0-9A-Fa-f]{6}$'),
    -- Un cuaderno cifrado no puede estar en el indice: seria filtrar por la puerta
    -- de atras lo que se ha cifrado por la de delante.
    CONSTRAINT notebook_cifrado_no_indexa CHECK (NOT (encrypted AND indexable)),
    -- Publico exige token, y el token solo tiene sentido si es publico.
    CONSTRAINT notebook_publico_con_token
        CHECK ((visibility = 'public') = (length(public_token) > 0))
);

CREATE INDEX notebook_owner_idx ON notebooks (tenant, owner_sub, position) WHERE deleted_at IS NULL;
CREATE INDEX notebook_parent_idx ON notebooks (tenant, parent_id) WHERE deleted_at IS NULL;
CREATE INDEX notebook_room_idx ON notebooks (tenant, room_id) WHERE room_id <> '' AND deleted_at IS NULL;
CREATE UNIQUE INDEX notebook_public_token ON notebooks (public_token) WHERE public_token <> '';

-- ---------------------------------------------------------------------------
-- Notas: el contenido heterogeneo del lienzo. `kind` decide como se pinta y que
-- hay en `data`; `content` es SIEMPRE el texto legible (lo que busca el indice y
-- lo que lee un lector de pantalla), tambien en las notas que ademas llevan un
-- objeto incrustado.
-- ---------------------------------------------------------------------------
CREATE TABLE notes (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    notebook_id     uuid        NOT NULL REFERENCES notebooks (id) ON DELETE CASCADE,
    owner_sub       text        NOT NULL,
    kind            text        NOT NULL DEFAULT 'text'
                        CHECK (kind IN ('text', 'todo', 'image', 'file', 'link', 'color',
                                        'launcher', 'crossref', 'doc', 'drive', 'event', 'room')),
    title           text        NOT NULL DEFAULT '',
    -- HTML ya saneado por el servicio (allowlist), o texto plano segun el tipo.
    content         text        NOT NULL DEFAULT '',
    -- Criptograma "encv1:<base64>" producido por el NAVEGADOR cuando el cuaderno
    -- esta cifrado. Con el, title/content van vacios: el servidor no los conoce.
    content_cipher  text        NOT NULL DEFAULT '',
    -- Carga util por tipo (url, mime, tamano, id de documento, color...).
    data            jsonb       NOT NULL DEFAULT '{}'::jsonb,
    -- Disposicion. x/y valen en el lienzo libre; col/position en columnas y lista.
    x               integer     NOT NULL DEFAULT 0,
    y               integer     NOT NULL DEFAULT 0,
    col             integer     NOT NULL DEFAULT 0,
    position        integer     NOT NULL DEFAULT 0,
    width           integer     NOT NULL DEFAULT 260,
    -- Agrupacion/anidamiento: una nota puede colgar de otra (grupo plegable).
    group_id        uuid        REFERENCES notes (id) ON DELETE SET NULL,
    collapsed       boolean     NOT NULL DEFAULT false,
    -- Estado de tarea (kind='todo'), y fijada arriba del todo.
    checked         boolean     NOT NULL DEFAULT false,
    pinned          boolean     NOT NULL DEFAULT false,
    -- Sobreescriben lo del cuaderno cuando no son NULL (herencia por COALESCE).
    visibility      text        CHECK (visibility IN ('private', 'personal', 'organization', 'public')),
    indexable       boolean,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz,
    CONSTRAINT note_title_len CHECK (length(title) <= 400),
    CONSTRAINT note_content_len CHECK (length(content) <= 200000),
    CONSTRAINT note_cipher_len CHECK (length(content_cipher) <= 400000),
    CONSTRAINT note_width CHECK (width BETWEEN 80 AND 1200),
    -- O va en claro o va cifrada, nunca las dos cosas: dos copias del mismo texto
    -- con una de ellas legible es exactamente el fallo que el cifrado evita.
    CONSTRAINT note_o_claro_o_cifrado
        CHECK (content_cipher = '' OR (content = '' AND title = ''))
);

CREATE INDEX note_notebook_idx ON notes (tenant, notebook_id, col, position) WHERE deleted_at IS NULL;
CREATE INDEX note_owner_idx ON notes (tenant, owner_sub, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX note_group_idx ON notes (tenant, group_id) WHERE group_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX note_papelera_idx ON notes (tenant, owner_sub, deleted_at) WHERE deleted_at IS NOT NULL;

-- Indice de texto completo. `simple` y no un idioma concreto: la suite es de
-- cinco idiomas y elegir uno degradaria los otros cuatro; el prefijo (:*) da el
-- comportamiento de "buscar mientras escribes" que se espera.
ALTER TABLE notes ADD COLUMN search tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(content, '')), 'B')
    ) STORED;
CREATE INDEX note_search_idx ON notes USING GIN (search);

-- ---------------------------------------------------------------------------
-- Etiquetas con estado. Una etiqueta ("Prioridad") agrupa estados ("Alta",
-- "Media", "Baja"); marcar un estado sustituye al anterior DE LA MISMA etiqueta,
-- que es lo que la distingue de una etiqueta suelta.
-- ---------------------------------------------------------------------------
CREATE TABLE tags (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    owner_sub   text        NOT NULL,
    name        text        NOT NULL,
    position    integer     NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT tag_name_not_blank CHECK (length(name) > 0),
    CONSTRAINT tag_name_len CHECK (length(name) <= 60)
);
CREATE UNIQUE INDEX tag_unique_name ON tags (tenant, owner_sub, lower(name));

CREATE TABLE tag_states (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    tag_id      uuid        NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    name        text        NOT NULL,
    emoji       text        NOT NULL DEFAULT '',
    color       text        NOT NULL DEFAULT '#565b62',
    -- Atajo de teclado de un solo caracter (BasKet marca estados con la fila de
    -- numeros). Vacio = sin atajo.
    shortcut    text        NOT NULL DEFAULT '',
    position    integer     NOT NULL DEFAULT 0,
    CONSTRAINT tag_state_name_not_blank CHECK (length(name) > 0),
    CONSTRAINT tag_state_name_len CHECK (length(name) <= 60),
    CONSTRAINT tag_state_color_format CHECK (color ~ '^#[0-9A-Fa-f]{6}$'),
    CONSTRAINT tag_state_shortcut_len CHECK (length(shortcut) <= 1)
);
CREATE INDEX tag_state_tag_idx ON tag_states (tenant, tag_id, position);

CREATE TABLE note_tag_states (
    tenant       text NOT NULL,
    note_id      uuid NOT NULL REFERENCES notes (id) ON DELETE CASCADE,
    tag_id       uuid NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    tag_state_id uuid NOT NULL REFERENCES tag_states (id) ON DELETE CASCADE,
    PRIMARY KEY (note_id, tag_id)
);
CREATE INDEX note_tag_state_idx ON note_tag_states (tenant, tag_state_id);

-- ---------------------------------------------------------------------------
-- Enlaces a objetos de la suite. UNA sola tabla para todos los destinos, que es
-- lo que permite responder "¿que notas cuelgan de este evento?" con una consulta
-- indexada en vez de recorrer el jsonb de cada nota.
--
-- La referencia con Calendar es BIDIRECCIONAL y vive AQUI, una sola vez: la ficha
-- del evento la lee por `target_kind='calendar_event'` desde la superficie de
-- Calendar. Duplicarla en Calendar daria dos verdades que se desincronizan al
-- primer borrado.
-- ---------------------------------------------------------------------------
CREATE TABLE note_links (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    note_id     uuid        NOT NULL REFERENCES notes (id) ON DELETE CASCADE,
    target_kind text        NOT NULL
                    CHECK (target_kind IN ('calendar_event', 'room_channel', 'room_message',
                                           'docs_document', 'drive_file', 'notebook', 'note')),
    target_id   text        NOT NULL,
    -- Rotulo cacheado del destino (titulo del evento, nombre del fichero). Es una
    -- COPIA para poder pintar la tarjeta sin llamar a la otra app en cada render;
    -- se refresca al abrir la nota.
    label       text        NOT NULL DEFAULT '',
    url         text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT note_link_target_not_blank CHECK (length(target_id) > 0),
    CONSTRAINT note_link_len CHECK (length(target_id) <= 400 AND length(label) <= 400 AND length(url) <= 2048)
);
CREATE UNIQUE INDEX note_link_unique ON note_links (note_id, target_kind, target_id);
-- La consulta inversa (del objeto a sus notas): la que usa la ficha del evento.
CREATE INDEX note_link_target_idx ON note_links (tenant, target_kind, target_id);

-- ---------------------------------------------------------------------------
-- RLS deny-by-default por tenant.
-- ---------------------------------------------------------------------------
ALTER TABLE notebooks       ENABLE ROW LEVEL SECURITY;
ALTER TABLE notes           ENABLE ROW LEVEL SECURITY;
ALTER TABLE tags            ENABLE ROW LEVEL SECURITY;
ALTER TABLE tag_states      ENABLE ROW LEVEL SECURITY;
ALTER TABLE note_tag_states ENABLE ROW LEVEL SECURITY;
ALTER TABLE note_links      ENABLE ROW LEVEL SECURITY;

ALTER TABLE notebooks       FORCE ROW LEVEL SECURITY;
ALTER TABLE notes           FORCE ROW LEVEL SECURITY;
ALTER TABLE tags            FORCE ROW LEVEL SECURITY;
ALTER TABLE tag_states      FORCE ROW LEVEL SECURITY;
ALTER TABLE note_tag_states FORCE ROW LEVEL SECURITY;
ALTER TABLE note_links      FORCE ROW LEVEL SECURITY;

CREATE POLICY notebooks_tenant_isolation ON notebooks
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY notes_tenant_isolation ON notes
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY tags_tenant_isolation ON tags
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY tag_states_tenant_isolation ON tag_states
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY note_tag_states_tenant_isolation ON note_tag_states
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY note_links_tenant_isolation ON note_links
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
