-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- `sizing_alerts` se quedó sin política de fusión.
--
-- La 0028 dio política a todas las tablas con columna `tenant`… recorriendo las
-- que existían ENTONCES. `sizing_alerts` llegó en la 0033, cinco migraciones
-- después, con su aislamiento por tenant y su ventana de mantenimiento, pero sin
-- esta: bajo FORCE ROW LEVEL SECURITY el motor de fusión no vería sus filas, y el
-- preflight abortaría la fusión ENTERA de dos organizaciones señalando una tabla
-- que nadie relaciona con la migración que la creó.
--
-- Va suelta y con el nombre de la tabla escrito, no con otro barrido dinámico
-- como el de la 0028: el gate que caza esto (`platform/pkg/migcheck`) lee el SQL
-- como texto, así que un `EXECUTE format(...)` le resulta invisible y la tabla
-- siguiente volvería a colarse igual.

DROP POLICY IF EXISTS sizing_alerts_platform_merge ON sizing_alerts;

CREATE POLICY sizing_alerts_platform_merge ON sizing_alerts
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
