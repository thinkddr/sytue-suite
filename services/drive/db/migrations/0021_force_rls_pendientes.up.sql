-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- FORCE, no solo ENABLE. Ver services/cloud/db/migrations/0041 para el porqué:
-- `ENABLE ROW LEVEL SECURITY` NO se aplica al dueño de la tabla, así que el rol
-- propietario ve las filas de todos los tenants. En estas cuatro hay unidades de
-- equipo y versiones de ficheros, o sea contenido de cliente.
ALTER TABLE favorites          FORCE ROW LEVEL SECURITY;
ALTER TABLE file_versions      FORCE ROW LEVEL SECURITY;
ALTER TABLE team_drives        FORCE ROW LEVEL SECURITY;
ALTER TABLE team_drive_members FORCE ROW LEVEL SECURITY;
