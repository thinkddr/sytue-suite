-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Wave 6: servicio de cómputo gestionado 'job' (batchv1.Job run-to-completion,
-- paridad Scaleway Jobs). Amplía el CHECK de managed_services.kind para admitir
-- 'job' junto a los tipos existentes. El CHECK inline de 0001 quedó con el nombre
-- autogenerado managed_services_kind_check.

ALTER TABLE managed_services DROP CONSTRAINT IF EXISTS managed_services_kind_check;
ALTER TABLE managed_services ADD CONSTRAINT managed_services_kind_check
    CHECK (kind IN ('postgres', 'storage', 'functions', 'cron', 'job', 'auth'));
