-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira las personas de otra organización.
-- sytue:destructive-ok deshace 0036: sin la marca, sus filas de channel_members se
-- quedan como las de cualquier miembro y las verjas que exigen equipo las cierran.

DROP POLICY IF EXISTS channel_members_externos_maintenance ON channel_members;
DROP INDEX IF EXISTS channel_members_externos;
ALTER TABLE channel_members DROP COLUMN externo;
