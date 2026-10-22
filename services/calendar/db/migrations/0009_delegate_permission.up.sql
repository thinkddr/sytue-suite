-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Delegación de agenda: extiende el ACL de calendarios (0007_calendar_shares) con un
-- tercer nivel de permiso 'manage' (delegar). Un delegado es como un editor (ver +
-- crear/editar/borrar eventos del calendario), pero la relación se marca como
-- DELEGACIÓN: el asistente gestiona la agenda del dueño. Reutiliza toda la
-- maquinaria de shares (misma tabla, mismos endpoints, misma verja CalendarAccess);
-- solo cambia el conjunto de permisos admitidos y que la escritura acepta writer O
-- manage. La delegación solo la concede el DUEÑO de la agenda (como cualquier share).

ALTER TABLE calendar_shares
    DROP CONSTRAINT IF EXISTS calendar_shares_permission_check;

ALTER TABLE calendar_shares
    ADD CONSTRAINT calendar_shares_permission_check
    CHECK (permission IN ('reader', 'writer', 'manage'));
