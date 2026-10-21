-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Esquema base de SyERP. La decisión que ordena todo lo demás: NO hay una tabla
-- por módulo. Un ERP «con todos los módulos» escrito a tabla por módulo son
-- doscientas migraciones y doscientos CRUD que solo se diferencian en los
-- nombres de las columnas — y cada módulo nuevo vuelve a pedir una migración,
-- un despliegue y una ventana de mantenimiento.
--
-- Aquí el catálogo de modelos es DATO (internal/catalogo/modulos/*.json) y la
-- maestra vive en `records`, con los campos declarados en `data` (jsonb). Añadir
-- Flota, Calidad o Alquiler es añadir un fichero JSON: sin DDL, sin despliegue
-- de base de datos, sin bloqueo de tabla.
--
-- LO QUE NO VA EN JSONB, y es la otra mitad de la decisión: EL DINERO. La
-- contabilidad tiene tablas propias, columnas tipadas y restricciones de
-- verdad (`moves`/`move_lines`), porque un asiento descuadrado que solo
-- detectaba el código de la aplicación es exactamente el fallo que nadie ve
-- hasta que lo ve el auditor. El cuadre lo impone Postgres al confirmar la
-- transacción, no una función de Go que alguien puede olvidarse de llamar.
--
-- Importes en CÉNTIMOS (bigint). Sin `float` en ninguna columna de dinero:
-- 0,1 + 0,2 no es 0,3 y una factura no puede depender de eso.
--
-- Clean-room: modelo propio. Que un ERP tenga clientes, facturas y asientos por
-- partida doble son ideas —y contabilidad de 1494—, no código con licencia: no
-- se ha consultado ni portado fuente de terceros (CONVENTIONS §12).
--
-- Multi-tenant: cada fila lleva `tenant` (slug). RLS deny-by-default aísla por
-- la variable de sesión `app.tenant` (la fija pg.SetTenant en cada transacción).
-- RLS separa ORGANIZACIONES; separar personas dentro de una es trabajo de las
-- cláusulas de visibilidad del store.

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ---------------------------------------------------------------------------
-- records — toda la maestra de todos los módulos: contactos, productos,
-- oportunidades, pedidos, albaranes, empleados, vehículos, tickets…
--
-- `model` es la clave del modelo en el catálogo embebido. `data` son sus campos
-- declarados. `etiqueta` es el nombre legible YA RESUELTO (el campo `titulo` del
-- modelo): se guarda desnormalizado a propósito, porque es lo que se busca y lo
-- que se pinta al referenciar la fila desde otra —resolverlo leyendo el jsonb en
-- cada consulta convierte cada lista en un escaneo.
-- ---------------------------------------------------------------------------
CREATE TABLE records (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant     text        NOT NULL,
    model      text        NOT NULL,
    -- Referencia legible del documento (PV-2026-00042). Vacía en los modelos sin
    -- secuencia (un contacto no se numera). No es la clave: la clave es `id`.
    ref        text        NOT NULL DEFAULT '',
    etiqueta   text        NOT NULL DEFAULT '',
    data       jsonb       NOT NULL DEFAULT '{}'::jsonb,
    -- Estado del flujo, declarado por el modelo. Fuera del jsonb porque se filtra
    -- por él en CADA lista y porque el índice de listado lo necesita.
    estado     text        NOT NULL DEFAULT '',
    owner_sub  text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,

    -- Dos documentos del mismo modelo no pueden compartir referencia. Parcial
    -- sobre ref <> '': los modelos sin secuencia comparten la cadena vacía y no
    -- deben chocar entre sí.
    CONSTRAINT records_data_es_objeto CHECK (jsonb_typeof(data) = 'object')
);

CREATE UNIQUE INDEX records_ref_unica ON records (tenant, model, ref) WHERE ref <> '';

-- Listado de un modelo, paginado por keyset (created_at, id) DESC, sin N+1.
CREATE INDEX records_listado ON records (tenant, model, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;
-- Filtro por estado dentro del modelo (el kanban del módulo).
CREATE INDEX records_estado ON records (tenant, model, estado) WHERE deleted_at IS NULL;
-- Búsqueda por nombre legible. Trigram y no prefijo: quien busca "acme" espera
-- encontrar "Suministros Acme S.L.", y un índice de prefijo no lo da.
CREATE INDEX records_etiqueta_trgm ON records USING gin (etiqueta gin_trgm_ops);
-- Filtros por campo declarado (data @> '{"cliente":"…"}'). jsonb_path_ops es la
-- mitad de grande que el GIN por defecto y cubre justo el operador @>.
CREATE INDEX records_data ON records USING gin (data jsonb_path_ops);

-- ---------------------------------------------------------------------------
-- moves / move_lines — el libro diario. Partida doble de verdad.
-- ---------------------------------------------------------------------------
CREATE TABLE moves (
    id        uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant    text        NOT NULL,
    diario    text        NOT NULL,
    ref       text        NOT NULL,
    fecha     date        NOT NULL,
    concepto  text        NOT NULL DEFAULT '',
    moneda    char(3)     NOT NULL DEFAULT 'EUR',
    -- El registro que lo originó (la factura). Nulo en un asiento manual.
    origen_id uuid        REFERENCES records (id) ON DELETE SET NULL,
    estado    text        NOT NULL DEFAULT 'borrador'
                          CHECK (estado IN ('borrador', 'asentado', 'anulado')),
    owner_sub text        NOT NULL,
    posted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT moves_ref_unica UNIQUE (tenant, diario, ref),
    -- Asentado implica fecha de asiento: es el dato que hace legal al resto.
    CONSTRAINT moves_asentado_con_fecha CHECK ((estado <> 'asentado') OR (posted_at IS NOT NULL))
);

CREATE INDEX moves_libro ON moves (tenant, fecha DESC, id DESC);
-- Un documento genera UN asiento. Asentar dos veces la misma factura duplica
-- ingresos, y el sitio donde eso se impide de verdad es aquí: un reintento del
-- cliente, dos pestañas o una carrera entre réplicas se estrellan contra el
-- índice en vez de contra un `if` que alguien puede reordenar. Anulado queda
-- fuera: tras anular, se puede volver a asentar.
CREATE UNIQUE INDEX moves_un_asiento_por_documento ON moves (tenant, origen_id)
    WHERE origen_id IS NOT NULL AND estado <> 'anulado';

CREATE TABLE move_lines (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    move_id    uuid        NOT NULL REFERENCES moves (id) ON DELETE CASCADE,
    tenant     text        NOT NULL,
    -- Código de cuenta del plan contable (430000, 700000, 477000…). Texto: el
    -- plan es del país y de la organización, no nuestro.
    cuenta     text        NOT NULL,
    concepto   text        NOT NULL DEFAULT '',
    debe       bigint      NOT NULL DEFAULT 0 CHECK (debe  >= 0),
    haber      bigint      NOT NULL DEFAULT 0 CHECK (haber >= 0),
    tercero_id uuid        REFERENCES records (id) ON DELETE SET NULL,

    -- Un apunte va a un lado o al otro. Con las dos columnas a cero la línea no
    -- dice nada, así que tampoco vale.
    CONSTRAINT move_lines_un_solo_lado CHECK (
        (debe = 0) <> (haber = 0)
    )
);

CREATE INDEX move_lines_por_asiento ON move_lines (move_id);
CREATE INDEX move_lines_mayor ON move_lines (tenant, cuenta);

-- ---------------------------------------------------------------------------
-- El cuadre, en la base de datos.
--
-- DEFERRABLE INITIALLY DEFERRED: se comprueba AL CONFIRMAR, no línea a línea —
-- si no, insertar el primer apunte de un asiento de dos siempre fallaría.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION syerp_asiento_cuadrado() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    v_move   uuid := COALESCE(NEW.move_id, OLD.move_id);
    v_debe   bigint;
    v_haber  bigint;
BEGIN
    -- El asiento puede haberse borrado en la misma transacción (ON DELETE
    -- CASCADE): entonces no hay nada que cuadrar.
    IF NOT EXISTS (SELECT 1 FROM moves WHERE id = v_move) THEN
        RETURN NULL;
    END IF;
    SELECT COALESCE(sum(debe), 0), COALESCE(sum(haber), 0)
      INTO v_debe, v_haber
      FROM move_lines WHERE move_id = v_move;
    IF v_debe <> v_haber THEN
        RAISE EXCEPTION 'asiento descuadrado: debe=% haber=%', v_debe, v_haber
            USING ERRCODE = 'check_violation';
    END IF;
    -- Un asiento sin apuntes tampoco es un asiento.
    IF v_debe = 0 AND v_haber = 0 THEN
        RAISE EXCEPTION 'asiento sin apuntes' USING ERRCODE = 'check_violation';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER move_lines_cuadre
    AFTER INSERT OR UPDATE OR DELETE ON move_lines
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION syerp_asiento_cuadrado();

-- ---------------------------------------------------------------------------
-- Lo asentado no se toca.
--
-- Corregir un asiento asentado se hace con OTRO asiento que lo revierte. Esto no
-- es una preferencia de diseño: es lo que separa un libro contable de una hoja
-- de cálculo, y es lo que pregunta cualquier auditoría.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION syerp_asiento_inmutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.estado = 'asentado' THEN
        -- La ÚNICA transición permitida sobre un asiento asentado es anularlo,
        -- que deja la fila y sus apuntes donde estaban.
        IF TG_OP = 'UPDATE' AND NEW.estado = 'anulado'
           AND NEW.diario = OLD.diario AND NEW.ref = OLD.ref
           AND NEW.fecha = OLD.fecha AND NEW.moneda = OLD.moneda THEN
            RETURN NEW;
        END IF;
        RAISE EXCEPTION 'asiento % ya asentado: no se modifica ni se borra', OLD.ref
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$$;

CREATE TRIGGER moves_inmutables
    BEFORE UPDATE OR DELETE ON moves
    FOR EACH ROW EXECUTE FUNCTION syerp_asiento_inmutable();

CREATE OR REPLACE FUNCTION syerp_apunte_inmutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    -- Si el asiento se está borrando entero (o ya no existe), el CASCADE manda:
    -- lo que protege el libro es que el ASIENTO asentado no se pueda borrar.
    IF EXISTS (SELECT 1 FROM moves WHERE id = COALESCE(NEW.move_id, OLD.move_id)
                 AND estado = 'asentado') THEN
        RAISE EXCEPTION 'apunte de un asiento ya asentado: no se añade, ni se modifica, ni se borra'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$$;

-- INSERT TAMBIÉN, y es la mitad que se escapaba.
--
-- Con el trigger solo en UPDATE/DELETE, añadir apuntes a un asiento ya asentado
-- estaba permitido. Un apunte suelto lo paraba el cuadre por accidente, pero un
-- PAR equilibrado —un cargo y su abono— entraba sin que nada se quejara: dinero
-- nuevo dentro de un asiento cerrado, con su fecha y su número ya emitidos.
--
-- Lo encontró el test de integración al exigir que el fallo viniera de ESTE
-- trigger y no de cualquier error: hasta entonces pasaba en verde porque el
-- apunte que probaba descuadraba de casualidad.
--
-- Consecuencia en el código, y es deliberada: un asiento se escribe en
-- BORRADOR, se le ponen los apuntes y solo entonces se asienta. Es el orden que
-- sigue cualquier libro contable, y ahora la base lo impone en vez de confiarlo.
CREATE TRIGGER move_lines_inmutables
    BEFORE INSERT OR UPDATE OR DELETE ON move_lines
    FOR EACH ROW EXECUTE FUNCTION syerp_apunte_inmutable();

-- ---------------------------------------------------------------------------
-- sequences — numeración de documentos por organización, clave y periodo.
--
-- Se reserva con UPDATE … RETURNING dentro de la MISMA transacción que crea el
-- documento, así que dos peticiones simultáneas no pueden llevarse el mismo
-- número (la segunda espera al bloqueo de fila).
--
-- ponytail: si la transacción se deshace, ese número se pierde y la serie queda
-- con un hueco. Es el compromiso estándar y lo que permite numerar sin
-- serializar el servicio entero; una serie SIN huecos exige asignar el número al
-- confirmar y reintentar los choques.
-- ---------------------------------------------------------------------------
CREATE TABLE sequences (
    tenant  text   NOT NULL,
    clave   text   NOT NULL,
    periodo text   NOT NULL,
    -- ULTIMO número usado, no el siguiente: así reservar es un solo
    -- INSERT … ON CONFLICT DO UPDATE … RETURNING, y lo que devuelve ya es el
    -- número que le toca a este documento.
    ultimo  bigint NOT NULL DEFAULT 0 CHECK (ultimo >= 0),
    PRIMARY KEY (tenant, clave, periodo)
);

-- ---------------------------------------------------------------------------
-- modulos_activos — qué módulos ha encendido la organización.
--
-- El catálogo es global y viene embebido; lo que es de cada organización es
-- CUÁLES usa. Una empresa de servicios no quiere ver Fabricación en la barra
-- lateral, y esconderlo es la mitad de por qué esto se maneja mejor que la
-- competencia.
-- ---------------------------------------------------------------------------
CREATE TABLE modulos_activos (
    tenant     text        NOT NULL,
    modulo     text        NOT NULL,
    activo     boolean     NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, modulo)
);

-- ---------------------------------------------------------------------------
-- RLS: deny-by-default en las cinco tablas.
-- ---------------------------------------------------------------------------
ALTER TABLE records         ENABLE ROW LEVEL SECURITY;
ALTER TABLE moves           ENABLE ROW LEVEL SECURITY;
ALTER TABLE move_lines      ENABLE ROW LEVEL SECURITY;
ALTER TABLE sequences       ENABLE ROW LEVEL SECURITY;
ALTER TABLE modulos_activos ENABLE ROW LEVEL SECURITY;

-- FORCE además de ENABLE: sin él, el dueño de la tabla se salta la política, y
-- el usuario de la aplicación suele serlo.
ALTER TABLE records         FORCE ROW LEVEL SECURITY;
ALTER TABLE moves           FORCE ROW LEVEL SECURITY;
ALTER TABLE move_lines      FORCE ROW LEVEL SECURITY;
ALTER TABLE sequences       FORCE ROW LEVEL SECURITY;
ALTER TABLE modulos_activos FORCE ROW LEVEL SECURITY;

CREATE POLICY records_tenant_isolation ON records
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY moves_tenant_isolation ON moves
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY move_lines_tenant_isolation ON move_lines
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY sequences_tenant_isolation ON sequences
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY modulos_activos_tenant_isolation ON modulos_activos
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
