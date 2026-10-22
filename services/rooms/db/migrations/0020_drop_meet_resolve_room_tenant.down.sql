-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Se recrea tal cual estaba. Ojo: bajo FORCE ROW LEVEL SECURITY devuelve cero filas
-- (ver el .up.sql); se restaura por simetría de la migración, no porque sirva.
CREATE FUNCTION meet_resolve_room_tenant(p_name text)
RETURNS SETOF text
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
    SELECT tenant FROM public.rooms WHERE name = p_name LIMIT 1;
$$;
