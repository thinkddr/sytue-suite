-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Quita la guarda de escritura de 0015. No borra datos: la copia retenida y el estado
-- de cada fuente siguen; solo se vuelve a escribir sin mirar el estado, como antes.
DROP TRIGGER IF EXISTS hub_private_prefs_guard ON notification_prefs;
DROP TRIGGER IF EXISTS hub_private_notifications_guard ON notifications;
DROP FUNCTION IF EXISTS hub_private_row_guard();
DROP FUNCTION IF EXISTS hub_private_storage_frozen(text, text);
