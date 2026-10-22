-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Grupos admin (CRUD de Sytue Console) + claim `groups` del JWT que desbloquea el
-- share por-grupo de Sytue Drive. Las tablas `groups` y `group_members` YA existen
-- (mig 0001). Esta migración solo:
--   1. Añade `groups.description` (texto libre opcional): lo exige el contrato admin
--      (GET/POST/PATCH /admin/groups) y el modelo AdminGroup de la Console. NULL por
--      defecto ⇒ compatible con las filas existentes.
--   2. Índice group_members(user_id): GroupsForUser filtra por user_id en CADA
--      emisión de token (claim `groups`); la PK (group_id,user_id) no sirve ese
--      acceso. Evita Seq Scan a escala (espeja memberships_user_idx de mig 0001).
--
-- Sin RLS: Sytue ID es la FUENTE de identidad (mig 0001) y aísla por columna
-- org_id en cada consulta (no por políticas RLS de sesión, que sí usan los
-- servicios consumidores aguas abajo). Los métodos de store de grupos acotan
-- SIEMPRE por org_id, igual que users/memberships.
ALTER TABLE groups ADD COLUMN description text;

CREATE INDEX group_members_user_idx ON group_members (user_id);
