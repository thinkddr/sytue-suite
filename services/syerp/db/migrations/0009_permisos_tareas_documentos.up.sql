-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El ERP pasa a ser de toda la organización, con permisos por módulo al estilo
-- de Odoo, tareas con vencimiento sobre cualquier registro, documentos adjuntos
-- y los avisos por correo de las obligaciones.

-- Nivel de cada persona en cada módulo. Solo se guarda lo que difiere del nivel
-- por defecto del módulo: así cambiar un defecto alcanza a quien no se tocó.
-- Owner y admin de la organización son administradores de todo por su rol y no
-- tienen filas aquí.
CREATE TABLE permisos (
    tenant          text        NOT NULL,
    sub             text        NOT NULL,
    modulo          text        NOT NULL,
    nivel           text        NOT NULL,
    email           text        NOT NULL DEFAULT '',
    nombre          text        NOT NULL DEFAULT '',
    actualizado_por text        NOT NULL,
    actualizado_en  timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (tenant, sub, modulo),
    CONSTRAINT permisos_nivel CHECK (nivel IN ('ninguno', 'lectura', 'usuario', 'administrador'))
);

-- Quién ha entrado en el ERP, con su correo y sus roles de la organización tal
-- como venían en el token. Es a quien se puede avisar por correo sin pedirle nada
-- a Sytue ID: el aviso diario corre sin la sesión de nadie.
CREATE TABLE usuarios (
    tenant   text        NOT NULL,
    sub      text        NOT NULL,
    email    text        NOT NULL DEFAULT '',
    nombre   text        NOT NULL DEFAULT '',
    roles    text[]      NOT NULL DEFAULT '{}',
    visto_en timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (tenant, sub)
);

-- Tareas con fecha sobre un registro (las «actividades» de Odoo): subir la
-- factura de un gasto, pedirla al proveedor, llamar a un cliente. `registro` es
-- texto y no uuid porque también se cuelgan de una obligación («es_303:2026-T3»).
CREATE TABLE actividades (
    id                 uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant             text        NOT NULL,
    modelo             text        NOT NULL DEFAULT '',
    registro           text        NOT NULL DEFAULT '',
    tipo               text        NOT NULL,
    resumen            text        NOT NULL,
    notas              text        NOT NULL DEFAULT '',
    vence              date        NOT NULL,
    responsable_sub    text        NOT NULL DEFAULT '',
    responsable_nombre text        NOT NULL DEFAULT '',
    estado             text        NOT NULL DEFAULT 'pendiente',
    creada_por         text        NOT NULL,
    creada_en          timestamptz NOT NULL DEFAULT now(),
    hecha_en           timestamptz,

    CONSTRAINT actividades_estado CHECK (estado IN ('pendiente', 'hecha', 'cancelada')),
    CONSTRAINT actividades_tipo CHECK (tipo IN ('subir_documento', 'pedir_documento', 'llamada', 'correo', 'reunion', 'tarea'))
);

CREATE INDEX actividades_pendientes ON actividades (tenant, vence) WHERE estado = 'pendiente';
CREATE INDEX actividades_de_registro ON actividades (tenant, modelo, registro);

-- Documentos adjuntos: la factura en PDF de un gasto, el justificante de una
-- obligación presentada.
--
-- En la base y no en un almacén de objetos, a propósito: es un PDF de cientos de
-- kilobytes, entra en la copia de seguridad y en el aislamiento por organización
-- sin credenciales nuevas. El tope de 10 MiB lo pone la propia tabla.
CREATE TABLE adjuntos (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant     text        NOT NULL,
    modelo     text        NOT NULL,
    registro   text        NOT NULL,
    nombre     text        NOT NULL,
    tipo       text        NOT NULL,
    tamano     integer     NOT NULL,
    sha256     text        NOT NULL,
    contenido  bytea       NOT NULL,
    creado_por text        NOT NULL,
    creado_en  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT adjuntos_tamano CHECK (tamano > 0 AND tamano <= 10485760 AND octet_length(contenido) = tamano)
);

CREATE INDEX adjuntos_de_registro ON adjuntos (tenant, modelo, registro);

-- Un aviso por obligación, periodo y tipo, aunque haya varias réplicas o el
-- servicio se reinicie a media mañana: la clave primaria es el cerrojo.
CREATE TABLE avisos_enviados (
    tenant     text        NOT NULL,
    clave      text        NOT NULL,
    periodo    text        NOT NULL,
    tipo       text        NOT NULL,
    enviado_en timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (tenant, clave, periodo, tipo)
);

ALTER TABLE permisos ENABLE ROW LEVEL SECURITY;
ALTER TABLE permisos FORCE ROW LEVEL SECURITY;
ALTER TABLE usuarios ENABLE ROW LEVEL SECURITY;
ALTER TABLE usuarios FORCE ROW LEVEL SECURITY;
ALTER TABLE actividades ENABLE ROW LEVEL SECURITY;
ALTER TABLE actividades FORCE ROW LEVEL SECURITY;
ALTER TABLE adjuntos ENABLE ROW LEVEL SECURITY;
ALTER TABLE adjuntos FORCE ROW LEVEL SECURITY;
ALTER TABLE avisos_enviados ENABLE ROW LEVEL SECURITY;
ALTER TABLE avisos_enviados FORCE ROW LEVEL SECURITY;

CREATE POLICY permisos_tenant_isolation ON permisos
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY usuarios_tenant_isolation ON usuarios
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY actividades_tenant_isolation ON actividades
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY adjuntos_tenant_isolation ON adjuntos
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY avisos_enviados_tenant_isolation ON avisos_enviados
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Escritas literales, una por tabla: el gate de fusión lee el SQL sin ejecutarlo
-- (ver la 0006).
CREATE POLICY permisos_platform_merge ON permisos
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
CREATE POLICY usuarios_platform_merge ON usuarios
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
CREATE POLICY actividades_platform_merge ON actividades
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
CREATE POLICY adjuntos_platform_merge ON adjuntos
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
CREATE POLICY avisos_enviados_platform_merge ON avisos_enviados
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

-- El aviso diario necesita saber QUÉ organizaciones tienen perfil antes de fijar
-- el tenant de cada una. Solo lectura y solo de esta tabla: lo demás lo hace ya
-- dentro de la organización, con su RLS de siempre.
CREATE POLICY organizacion_maintenance ON organizacion
    FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on');
