-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Archivado de unidades de equipo: marca ORGANIZATIVA (no borra contenido ni
-- revoca membresía). NULL = activa; timestamp = archivada. Solo un manager la
-- conmuta (gateado en el servicio). El aislamiento y el resto de reglas no
-- cambian: una unidad archivada sigue existiendo y accesible a sus miembros.
ALTER TABLE team_drives ADD COLUMN archived_at timestamptz;
