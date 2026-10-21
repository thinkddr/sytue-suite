-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- FUSIÓN DE ORGANIZACIONES: política PERMISSIVE acotada a `app.merge='on'`.
--
-- La administración de plataforma puede fusionar dos organizaciones en una. Eso
-- reescribe el `tenant` de cada fila de este servicio, y bajo FORCE ROW LEVEL
-- SECURITY ninguna transacción normal puede hacerlo: la política de aislamiento
-- ni siquiera VE las filas del origen, así que un UPDATE sin esta política
-- afectaría a cero filas y la fusión se declararía completada sin mover nada.
--
-- El alcance es deliberadamente estrecho:
--   · Variable propia `app.merge`, no `app.maintenance`: las transacciones de
--     mantenimiento ya escritas no ganan alcance nuevo.
--   · Solo la fija `pg.InMergeTx`, que solo usa el motor de fusión de Sytue ID.
--   · Las transacciones de negocio nunca la fijan, así que el aislamiento por
--     tenant de este servicio queda exactamente igual que antes.
--
-- Se genera recorriendo las tablas con columna `tenant` en lugar de enumerarlas:
-- una tabla nueva sin política sería un agujero silencioso en la fusión, y el
-- preflight del motor la detectaría, pero es mejor que no ocurra. Aquí cubre las
-- siete: records, moves, move_lines, sequences, modulos_activos, conexiones y
-- eventos_entrantes.
--
-- LO QUE SE MUEVE EN UNA FUSIÓN es todo el ERP de la organización absorbida:
-- sus clientes, sus documentos y SU LIBRO DIARIO. Los asientos son inmutables
-- (trigger `moves_inmutables`), pero eso prohíbe cambiarles diario, referencia,
-- fecha o moneda —no el tenant—, así que la fusión los lleva enteros sin
-- reescribir contabilidad. Un asiento que se quedara con el tenant viejo
-- desaparecería del balance de la organización resultante, que es exactamente el
-- descuadre que nadie sabría de dónde viene.

DO $$
DECLARE
    t text;
BEGIN
    FOR t IN
        SELECT c.relname
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'tenant' AND a.attnum > 0
        WHERE c.relkind = 'r' AND n.nspname = current_schema()
        ORDER BY c.relname
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS %I ON %I', t || '_platform_merge', t);
        EXECUTE format(
            'CREATE POLICY %I ON %I USING (current_setting(''app.merge'', true) = ''on'') '
            'WITH CHECK (current_setting(''app.merge'', true) = ''on'')',
            t || '_platform_merge', t);
    END LOOP;
END $$;
