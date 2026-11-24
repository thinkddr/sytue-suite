-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Concede a los roles de servicio `SET` sobre los parámetros `app.*` que las
-- migraciones dejan escritos en la definición de una función. Sin esto, un servicio
-- que entra con su propio rol (bin/separar-roles.sh, INF-01) no puede aplicar sus
-- migraciones.
--
-- POR QUÉ. Las funciones del almacenamiento privado y de la admisión de ámbitos
-- llevan cláusulas como `SET app.maintenance = 'on'`, y desde PostgreSQL 15 escribir
-- un parámetro propio (un «placeholder», sin extensión que lo declare) en la
-- definición de una función —CREATE FUNCTION … SET, ALTER FUNCTION … SET o RESET—
-- exige ser superusuario o tener `GRANT SET ON PARAMETER` sobre él. En CI las
-- migraciones corren como superusuario y pasaban; en el piloto cada servicio entra
-- con `sytue_<svc>`, sin SUPERUSER, y el 2026-11-16 (v3.20.3) doce servicios se
-- quedaron con la migración sucia y en CrashLoopBackOff con
-- `permission denied to set parameter "app.maintenance"`. Ver docs/operations.md,
-- «Incidencia: los roles de servicio no podían fijar los parámetros app.*».
--
-- QUÉ DA Y QUÉ NO. Solo deja escribir esos parámetros en la definición de una
-- función (o de un rol o una base). En su sesión, cualquier rol ya puede
-- `SET app.maintenance` o `set_config('app.maintenance', …)`: el privilegio no le
-- abre nada que no tuviera. Ejecutar la función tampoco lo necesita.
--
-- LA LISTA VIVE AQUÍ Y SOLO AQUÍ. La usan, leyendo este fichero:
--   · bin/separar-roles.sh (crear-roles y traspasar) y bin/deploy.sh en el piloto
--     —y con él bin/actualizar-suite.sh, que lanza el deploy.sh de la versión nueva—;
--   · deploy/compose/postgres/init/01-databases.sh en dev, montado en el contenedor;
--   · platform/pkg/pgtest (MigrarComoDueno), que aplica las migraciones de cada
--     servicio con un dueño sin SUPERUSER que solo tiene lo que concede esto;
--   · y platform/pkg/migcheck (TestParametrosDeFuncionConcedidos), que falla si una
--     migración o un fragmento de platform/pkg/pg fija con SET de función un
--     parámetro que no esté aquí. Una función nueva con `SET app.<otro>`: se añade a
--     `parametros` y nada más.
--
-- A QUIÉN, sin superusuarios (no les hace falta) ni roles del sistema:
--   · al dueño de cada base `sytue_*`: es quien migra, porque deploy.sh escribe el
--     DSN de cada base con su dueño;
--   · al rol que se llama como una base `sytue_*` aunque aún no sea su dueño
--     (`separar-roles.sh crear-roles` lo crea antes de traspasar);
--   · a los roles de aplicación NOLOGIN `*_app` (docs_app, integrations_app), que el
--     servicio asume con SET ROLE;
--   · y a los que se nombren en `sytue.roles`, separados por comas, fijado en la
--     MISMA sesión antes de este fichero (pgtest, dev y separar-roles.sh lo usan).
--
-- Idempotente: conceder lo que ya está concedido no cambia nada. Un solo DO: o todo
-- o nada, salvo un rol hallado que otra sesión borra a mitad, que se salta. Lo
-- ejecuta un superusuario, que es quien puede conceder sobre parámetros.
DO $parametros$
DECLARE
  -- En orden alfabético. app.merge no lo lleva hoy ninguna función en su SET (lo
  -- fijan la fusión y la bajada 0008 de Notepads en su sesión, que no lo exige),
  -- pero es el siguiente candidato y concederlo no abre nada: va de antemano.
  parametros CONSTANT text[] := ARRAY[
    'app.docs_definer',
    'app.maintenance',
    'app.merge',
    'app.scope_admission_internal'
  ];
  nombrados text[] := coalesce(string_to_array(nullif(current_setting('sytue.roles', true), ''), ','), '{}');
  roles text[];
  rol text;
  falta text;
BEGIN
  IF EXISTS (SELECT FROM unnest(parametros) p WHERE p !~ '^[a-z_]+\.[a-z_][a-z0-9_]*$') THEN
    RAISE EXCEPTION 'conceder-parametros: hay un nombre de parámetro no válido en %', parametros;
  END IF;

  -- Un rol nombrado que no existe es un error de quien llama, no algo que saltar.
  SELECT string_agg(n, ', ') INTO falta
    FROM unnest(nombrados) n WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = n);
  IF falta IS NOT NULL THEN
    RAISE EXCEPTION 'conceder-parametros: no existe el rol %', falta;
  END IF;

  SELECT coalesce(array_agg(r.rolname::text ORDER BY r.rolname), '{}') INTO roles
    FROM pg_roles r
   WHERE NOT r.rolsuper AND r.rolname !~ '^pg_'
     AND (r.oid IN (SELECT datdba FROM pg_database WHERE datname LIKE 'sytue\_%')
          OR r.rolname IN (SELECT datname FROM pg_database WHERE datname LIKE 'sytue\_%')
          OR (NOT r.rolcanlogin AND NOT r.rolbypassrls AND r.rolname LIKE '%\_app')
          OR r.rolname = ANY (nombrados));

  IF cardinality(roles) = 0 THEN
    RAISE NOTICE 'parámetros de funciones: ningún rol de servicio sin SUPERUSER, nada que conceder';
    RETURN;
  END IF;

  -- GRANT SET ON PARAMETER llegó en la 15. Antes, un rol sin SUPERUSER no puede
  -- escribir un parámetro propio en una función de ninguna manera: mejor pararlo
  -- aquí que en el CrashLoopBackOff de doce servicios.
  IF current_setting('server_version_num')::int < 150000 THEN
    RAISE EXCEPTION 'PostgreSQL % no tiene GRANT SET ON PARAMETER (llegó en la 15): % no podrían aplicar las migraciones que fijan %',
      current_setting('server_version'), array_to_string(roles, ', '), array_to_string(parametros, ', ');
  END IF;

  -- Rol a rol: uno HALLADO que ya no existe (otra sesión lo borró entre la lista y el
  -- GRANT; en CI, las pruebas que comparten servidor crean y borran roles `*_app`)
  -- no tiene nada que conceder. Uno NOMBRADO que desaparece sigue siendo un error.
  FOREACH rol IN ARRAY roles LOOP
    BEGIN
      EXECUTE format('GRANT SET ON PARAMETER %s TO %I', array_to_string(parametros, ', '), rol);
    EXCEPTION WHEN undefined_object THEN
      IF rol = ANY (nombrados) THEN
        RAISE;
      END IF;
    END;
  END LOOP;
  RAISE NOTICE 'parámetros de funciones: SET sobre % para %',
    array_to_string(parametros, ', '), array_to_string(roles, ', ');
END
$parametros$;
