-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP TABLE IF EXISTS buzones_olvidados;
DROP POLICY IF EXISTS notification_prefs_destinatario_lee ON notification_prefs;
DROP TRIGGER IF EXISTS notifications_solo_se_marca ON notifications;
DROP FUNCTION IF EXISTS notifications_solo_se_marca();
