-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Vuelve el aislamiento por organización a secas: los avisos de otra organización
-- dejan de verse en la campana (siguen en la base).
DROP POLICY IF EXISTS notifications_destinatario_marca ON notifications;
DROP POLICY IF EXISTS notifications_destinatario_lee ON notifications;
