-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- La política de fusión que le faltaba a `organizacion`.
--
-- POR QUÉ HACE FALTA UNA MIGRACIÓN NUEVA Y NO BASTABA LA 0003. Aquella recorre
-- las tablas con columna `tenant` y les crea su política, pero lo hace CUANDO SE
-- EJECUTA: cubrió las siete que existían entonces. `organizacion` nació en la
-- 0005, después, así que se quedó sin ella — y la propia 0003 avisaba de que eso
-- sería «un agujero silencioso en la fusión».
--
-- Lo que pasaría sin esto, y por eso no es cosmético: bajo FORCE ROW LEVEL
-- SECURITY la política de aislamiento ni siquiera VE las filas de la organización
-- de origen, así que el UPDATE que reescribe el `tenant` afectaría a cero filas.
-- El perfil de la organización absorbida —su NIF, su país, su forma jurídica y las
-- condiciones de las que sale su calendario de obligaciones— se quedaría atrás sin
-- que nada fallara.
--
-- ESCRITA LITERAL Y NO CON UN `DO $$ … format() $$` como la 0003, aunque el bucle
-- sea más corto. El gate `TestMigracionesNuevasTraenPoliticaDeFusion` lee el SQL
-- SIN EJECUTARLO: busca un `CREATE POLICY <nombre> ON <tabla>` que mencione
-- `app.merge` en su propia sentencia, y en un `format('… %I …')` el nombre de la
-- tabla no está escrito en ninguna parte. Con el bucle la política se crea igual
-- en la base y el gate sigue en rojo, que es como se pierde media hora.

CREATE POLICY organizacion_platform_merge ON organizacion
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
