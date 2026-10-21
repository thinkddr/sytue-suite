-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sytue Analytics — esquema inicial (ADR 0026).
--
-- Minimización como esquema: lo que no se escribe no se filtra. AQUÍ NO EXISTE
-- una columna para la IP, ni para la URL completa, ni para el Referer completo,
-- ni para el user-agent crudo, ni para el idioma. No es una política que alguien
-- deba respetar: no hay dónde ponerlos.
--
-- Cuatro piezas:
--   1) analytics_salts   — la sal diaria CIFRADA (keycrypto). Se borra al cambiar
--                          el día: sin ella no se puede recalcular ningún hash.
--   2) analytics_sites   — un sitio ES un hostname. El colector lo resuelve por
--                          la cabecera Host, así que no hay «site_id» que falsificar.
--   3) analytics_events  — el crudo, particionado por día, retención 48 h.
--   4) los agregados      — anónimos (sin identificador de visitante), viven años.
--
-- RLS en todas: ENABLE + FORCE + predicado explícito. Además de la política por
-- inquilino, dos flags de sesión (ADR §4):
--   app.ingest='on'       el colector: INSERT en eventos y SELECT de sitios/sal.
--                         NO puede leer eventos — el que escribe no puede leer.
--   app.maintenance='on'  rollup y purga, que son cross-tenant por naturaleza.

-- 1) La sal diaria ------------------------------------------------------------
-- Sin tenant a propósito: la sal es global y el site_id va DENTRO del HMAC, que
-- es lo que hace incorrelacionables a dos sitios distintos. `salt` guarda el
-- sobre "encv1:base64(...)" de platform/pkg/keycrypto: un volcado de PostgreSQL
-- sin la KEK del gestor de secretos no permite recalcular ni un solo hash.
CREATE TABLE analytics_salts (
    day        date        PRIMARY KEY,
    salt       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE analytics_salts ENABLE ROW LEVEL SECURITY;
ALTER TABLE analytics_salts FORCE ROW LEVEL SECURITY;
-- Ambos procesos la necesitan: el colector para hashear, el cron para rotarla.
-- Ninguna transacción de negocio fija estos flags, así que la API de lectura
-- (que corre con app.tenant) NO ve esta tabla en absoluto.
CREATE POLICY analytics_salts_service ON analytics_salts
    USING (current_setting('app.ingest', true) = 'on'
        OR current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.ingest', true) = 'on'
        OR current_setting('app.maintenance', true) = 'on');

-- 2) Sitios -------------------------------------------------------------------
-- host es UNIQUE global: un hostname pertenece a un solo inquilino, que es la
-- razón por la que identificar el sitio por la cabecera Host es seguro.
CREATE TABLE analytics_sites (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    host            text        NOT NULL UNIQUE CHECK (host = lower(host) AND length(host) BETWEEN 1 AND 253),
    -- Allowlist de nombres de evento declarados con data-sytue-event. Sin ella,
    -- data-sytue-event="pepe@correo.com" convierte el almacén en un canal de
    -- exfiltración. Los automáticos (pageview, outbound…) no pasan por aquí.
    event_allowlist text[]      NOT NULL DEFAULT '{}' CHECK (array_length(event_allowlist, 1) IS NULL OR array_length(event_allowlist, 1) <= 50),
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX analytics_sites_tenant_idx ON analytics_sites (tenant, host);

ALTER TABLE analytics_sites ENABLE ROW LEVEL SECURITY;
ALTER TABLE analytics_sites FORCE ROW LEVEL SECURITY;
CREATE POLICY analytics_sites_tenant ON analytics_sites
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
-- El colector resuelve Host -> sitio antes de saber de qué inquilino es: SOLO
-- lectura, y solo con el flag del colector.
CREATE POLICY analytics_sites_ingest ON analytics_sites FOR SELECT
    USING (current_setting('app.ingest', true) = 'on');
CREATE POLICY analytics_sites_maintenance ON analytics_sites
    USING (current_setting('app.maintenance', true) = 'on');

-- 3) El crudo -----------------------------------------------------------------
-- Particionada por RANGE en ts, una partición por día: la purga de millones de
-- filas es un DROP TABLE, sin vacuum ni WAL.
--
-- SIN ÍNDICES y SIN clave foránea a analytics_sites, a propósito: es una tabla
-- de escritura pura que solo lee el rollup, y este recorre la partición entera
-- del día (la poda por ts ya la limita). Cada índice sería coste por evento en
-- el camino más caliente de la plataforma para una tabla que vive 48 h.
CREATE TABLE analytics_events (
    tenant        text        NOT NULL,
    site_id       uuid        NOT NULL,
    ts            timestamptz NOT NULL,
    -- HMAC-SHA256(sal_del_día, site_id || ip || user_agent) truncado a 16 bytes.
    visitor       bytea       NOT NULL CHECK (length(visitor) = 16),
    name          text        NOT NULL CHECK (length(name) <= 50),
    path          text        NOT NULL CHECK (length(path) <= 255),
    referrer_host text        CHECK (referrer_host IS NULL OR length(referrer_host) <= 253),
    -- Detalle acotado del evento automático: host de destino (outbound) o
    -- extensión (download). Nunca una URL ni un nombre de fichero.
    target        text        CHECK (target IS NULL OR length(target) <= 64),
    country       text        NOT NULL DEFAULT 'ZZ' CHECK (length(country) = 2),
    browser       text        NOT NULL CHECK (length(browser) <= 24),
    os            text        NOT NULL CHECK (length(os) <= 24),
    device        text        NOT NULL CHECK (device IN ('desktop', 'mobile', 'tablet', 'other')),
    utm_source    text        CHECK (utm_source IS NULL OR length(utm_source) <= 64),
    utm_medium    text        CHECK (utm_medium IS NULL OR length(utm_medium) <= 64),
    utm_campaign  text        CHECK (utm_campaign IS NULL OR length(utm_campaign) <= 64),
    utm_content   text        CHECK (utm_content IS NULL OR length(utm_content) <= 64),
    utm_term      text        CHECK (utm_term IS NULL OR length(utm_term) <= 64),
    ref           text        CHECK (ref IS NULL OR length(ref) <= 64)
) PARTITION BY RANGE (ts);

ALTER TABLE analytics_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE analytics_events FORCE ROW LEVEL SECURITY;
CREATE POLICY analytics_events_tenant ON analytics_events
    USING (tenant = current_setting('app.tenant', true));
-- El colector inserta y NO PUEDE LEER: la política es FOR INSERT, así que el
-- flag app.ingest no abre ningún SELECT sobre esta tabla.
CREATE POLICY analytics_events_ingest ON analytics_events FOR INSERT
    WITH CHECK (current_setting('app.ingest', true) = 'on');
CREATE POLICY analytics_events_maintenance ON analytics_events
    USING (current_setting('app.maintenance', true) = 'on');

-- Particiones: el proceso de la plataforma NO es dueño de la tabla y por tanto
-- no puede hacerle DDL. Estas dos funciones son SECURITY DEFINER (corren como el
-- dueño) y es todo el DDL que el servicio puede ejecutar: no aceptan un nombre
-- de tabla, solo un número de días, así que no hay SQL dinámico que inyectar.
-- OJO: SECURITY DEFINER no salta la RLS bajo FORCE (tampoco lo pretende); aquí
-- sirve para el PRIVILEGIO de crear y retirar particiones, nada más.
CREATE FUNCTION analytics_ensure_partitions(days_ahead int)
RETURNS int LANGUAGE plpgsql SECURITY DEFINER SET search_path = public AS $fn$
DECLARE
    d       date;
    created int := 0;
BEGIN
    IF days_ahead < 0 OR days_ahead > 60 THEN
        RAISE EXCEPTION 'days_ahead fuera de rango: %', days_ahead;
    END IF;
    -- Desde ayer (una réplica que arranca tras un hueco puede recibir eventos
    -- con marca de tiempo del día anterior) hasta days_ahead por delante.
    FOR d IN SELECT generate_series(current_date - 1, current_date + days_ahead, interval '1 day')::date LOOP
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I PARTITION OF analytics_events FOR VALUES FROM (%L) TO (%L)',
            'analytics_events_' || to_char(d, 'YYYYMMDD'), d::timestamptz, (d + 1)::timestamptz);
        created := created + 1;
    END LOOP;
    RETURN created;
END;
$fn$;

CREATE FUNCTION analytics_expire_partitions(keep_days int)
RETURNS int LANGUAGE plpgsql SECURITY DEFINER SET search_path = public AS $fn$
DECLARE
    part    record;
    cutoff  date;
    removed int := 0;
BEGIN
    IF keep_days < 1 OR keep_days > 60 THEN
        RAISE EXCEPTION 'keep_days fuera de rango: %', keep_days;
    END IF;
    -- keep_days = 2 conserva hoy y ayer: ninguna fila supera nunca las 48 h.
    cutoff := current_date - (keep_days - 1);
    FOR part IN
        SELECT c.relname AS name
        FROM pg_class c
        JOIN pg_inherits i ON i.inhrelid = c.oid
        JOIN pg_class p ON p.oid = i.inhparent
        WHERE p.relname = 'analytics_events'
          AND c.relname ~ '^analytics_events_[0-9]{8}$'
          AND to_date(right(c.relname, 8), 'YYYYMMDD') < cutoff
    LOOP
        EXECUTE format('DROP TABLE IF EXISTS %I', part.name);
        removed := removed + 1;
    END LOOP;
    RETURN removed;
END;
$fn$;

-- 4) Agregados ----------------------------------------------------------------
-- No contienen NINGÚN identificador de visitante: son anónimos y por eso pueden
-- vivir años. Dos tablas de totales y no una porque LOS ÚNICOS NO SE SUMAN: el
-- día se calcula sobre el crudo, no sumando las 24 horas.
CREATE TABLE analytics_hourly (
    tenant    text        NOT NULL,
    site_id   uuid        NOT NULL REFERENCES analytics_sites(id) ON DELETE CASCADE,
    hour      timestamptz NOT NULL,
    pageviews bigint      NOT NULL DEFAULT 0,
    visitors  bigint      NOT NULL DEFAULT 0,
    events    bigint      NOT NULL DEFAULT 0,
    PRIMARY KEY (site_id, hour)
);

CREATE TABLE analytics_daily (
    tenant    text   NOT NULL,
    site_id   uuid   NOT NULL REFERENCES analytics_sites(id) ON DELETE CASCADE,
    day       date   NOT NULL,
    pageviews bigint NOT NULL DEFAULT 0,
    -- Únicos DEL DÍA. Un rango largo muestra la SUMA de únicos diarios, nunca un
    -- único mensual deduplicado: sin la sal de ayer eso no se puede calcular.
    visitors  bigint NOT NULL DEFAULT 0,
    -- Visitas reconstruidas en el rollup (mismo visitor, hueco < 30 min).
    visits    bigint NOT NULL DEFAULT 0,
    bounces   bigint NOT NULL DEFAULT 0,
    events    bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (site_id, day)
);

CREATE TABLE analytics_daily_breakdown (
    tenant    text   NOT NULL,
    site_id   uuid   NOT NULL REFERENCES analytics_sites(id) ON DELETE CASCADE,
    day       date   NOT NULL,
    dim       text   NOT NULL,
    -- Top-N por día más un cajón '(otras)': es lo que acota el crecimiento, ya
    -- que las rutas NO se normalizan automáticamente (adivinar es peor).
    value     text   NOT NULL,
    pageviews bigint NOT NULL DEFAULT 0,
    visitors  bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (site_id, day, dim, value)
);

ALTER TABLE analytics_hourly ENABLE ROW LEVEL SECURITY;
ALTER TABLE analytics_hourly FORCE ROW LEVEL SECURITY;
CREATE POLICY analytics_hourly_tenant ON analytics_hourly
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY analytics_hourly_maintenance ON analytics_hourly
    USING (current_setting('app.maintenance', true) = 'on');

ALTER TABLE analytics_daily ENABLE ROW LEVEL SECURITY;
ALTER TABLE analytics_daily FORCE ROW LEVEL SECURITY;
CREATE POLICY analytics_daily_tenant ON analytics_daily
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY analytics_daily_maintenance ON analytics_daily
    USING (current_setting('app.maintenance', true) = 'on');

ALTER TABLE analytics_daily_breakdown ENABLE ROW LEVEL SECURITY;
ALTER TABLE analytics_daily_breakdown FORCE ROW LEVEL SECURITY;
CREATE POLICY analytics_daily_breakdown_tenant ON analytics_daily_breakdown
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY analytics_daily_breakdown_maintenance ON analytics_daily_breakdown
    USING (current_setting('app.maintenance', true) = 'on');

-- Particiones iniciales: ayer, hoy y una semana por delante, para que el
-- colector pueda escribir desde el primer segundo aunque el cron aún no haya
-- corrido su primer ciclo.
SELECT analytics_ensure_partitions(7);
