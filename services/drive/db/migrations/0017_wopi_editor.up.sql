-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- EL EDITOR EXTERNO ES UN AJUSTE DE LA ORGANIZACIÓN, no del despliegue.
--
-- Estaba en una variable de entorno del servicio, así que conectar el Collabora de una
-- organización obligaba a editar el Helm y reiniciar Drive — y en una instalación con
-- varias organizaciones, todas compartían editor lo quisieran o no. Quién edita los
-- documentos de una empresa es decisión de esa empresa, y se toma desde la consola.
--
-- Una fila por organización: o tiene editor o no lo tiene.
CREATE TABLE wopi_editors (
    tenant text PRIMARY KEY,
    -- URL del `discovery.xml` del editor. Es lo ÚNICO que hay que pedir: de ahí salen
    -- las acciones por tipo de fichero, que es lo que nadie debería teclear a mano.
    discovery_url text NOT NULL,
    -- Nombre para reconocerlo en la consola («Collabora de la oficina»). Decoración: lo
    -- escribe quien lo conecta y no interviene en nada.
    label text NOT NULL DEFAULT '',
    -- Apagarlo sin perder la configuración: al volver a encenderlo no hay que buscar la
    -- URL otra vez. Borrar la fila sería la alternativa, y obliga a reconfigurar.
    enabled boolean NOT NULL DEFAULT true,
    -- Última comprobación del catálogo: la consola dice si responde y desde cuándo, en
    -- vez de dejar a quien lo conecta adivinando si funcionó.
    checked_at   timestamptz,
    check_ok     boolean NOT NULL DEFAULT false,
    check_error  text    NOT NULL DEFAULT '',
    -- Extensiones que declaró la última vez. Se guardan para poder enseñarlas sin
    -- volver a llamar al editor cada vez que alguien abre la pantalla.
    extensions text[] NOT NULL DEFAULT '{}',
    updated_by text        NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT wopi_editors_url_not_blank CHECK (length(discovery_url) > 0)
);

ALTER TABLE wopi_editors ENABLE ROW LEVEL SECURITY;
ALTER TABLE wopi_editors FORCE ROW LEVEL SECURITY;

CREATE POLICY wopi_editors_isolation ON wopi_editors
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- El host WOPI resuelve el editor SIN sesión de usuario: el editor llama de vuelta con
-- su token y ahí no hay JWT del que sacar la organización. Misma política acotada de
-- mantenimiento que usan los webhooks entrantes de Rooms y el MX de Mail.
CREATE POLICY wopi_editors_maintenance ON wopi_editors
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

-- Fusión de organizaciones (ver 0015).
CREATE POLICY wopi_editors_merge ON wopi_editors
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
