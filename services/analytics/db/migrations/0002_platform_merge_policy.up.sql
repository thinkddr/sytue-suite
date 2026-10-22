-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- FUSIÓN DE ORGANIZACIONES: política PERMISSIVE acotada a `app.merge='on'`.
--
-- Sytue Analytics era el ÚNICO servicio con datos por `tenant` que no participaba
-- en la fusión: al absorber una organización, sus sitios y sus históricos se
-- quedaban con el tenant viejo y desaparecían de la vista del cliente sin que
-- nada fallara. Las otras nueve bases tienen esta política desde su día uno; ver
-- services/mail/db/migrations/0010_platform_merge_policy.up.sql.
--
-- POR QUÉ HACE FALTA. Todas estas tablas llevan FORCE ROW LEVEL SECURITY, que
-- aplica las políticas TAMBIÉN al dueño de la tabla. Sin una política que abra la
-- escritura durante la fusión, la transacción no VE las filas del origen: el
-- `UPDATE ... SET tenant = destino WHERE tenant = origen` afecta a cero filas y
-- NO da error. La fusión se declararía completada sin haber movido nada.
--
-- El alcance es estrecho a propósito: variable propia `app.merge`, que solo fija
-- `pg.InMergeTx` y solo usa el motor de fusión de Sytue ID. Las transacciones de
-- negocio y las de mantenimiento (`app.tenant`, `app.ingest`, `app.maintenance`)
-- no ganan ni un permiso.
--
-- COLISIONES AL FUSIONAR: NINGUNA, y no por suerte.
--   · analytics_sites.host es UNIQUE GLOBAL, no por inquilino, así que dos
--     organizaciones no pueden tener el mismo sitio: no hay nada que chocar.
--   · Los agregados llevan PRIMARY KEY (site_id, …) con un uuid que viaja con su
--     sitio; al ser único no colisiona con el del destino.
--   · analytics_salts NO tiene columna `tenant` y por eso NO se mueve: la sal es
--     global y compartida. Moverla no significaría nada.
-- Es decir, esta base no necesita gancho `platform_merge_prepare`.

DO $$
DECLARE
    t text;
BEGIN
    FOR t IN
        SELECT c.relname
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'tenant' AND a.attnum > 0
        -- 'p' además de 'r', y NOT relispartition: `analytics_events` está
        -- PARTICIONADA POR DÍA y es la única tabla particionada de la suite.
        --
        -- La plantilla de los otros servicios filtra por relkind = 'r', que aquí
        -- haría justo lo contrario de lo que parece: saltarse la tabla madre (es
        -- 'p') y crear la política sobre las particiones diarias, que sí son 'r'.
        -- Eso funcionaría hoy y se rompería mañana, porque
        -- `analytics_ensure_partitions` crea una partición nueva cada día y
        -- nacería sin política. Cada mañana la fusión abortaría en el preflight.
        --
        -- Sobre la madre basta: PostgreSQL aplica sus políticas a las filas de
        -- todas sus particiones cuando se accede por ella, que es como accede el
        -- motor de fusión.
        WHERE c.relkind IN ('r', 'p')
          AND NOT c.relispartition
          AND n.nspname = current_schema()
        ORDER BY c.relname
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS %I ON %I', t || '_platform_merge', t);
        EXECUTE format(
            'CREATE POLICY %I ON %I USING (current_setting(''app.merge'', true) = ''on'') '
            'WITH CHECK (current_setting(''app.merge'', true) = ''on'')',
            t || '_platform_merge', t);
    END LOOP;
END $$;
