-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte 0012: restaura el CHECK de managed_services.kind sin 'job'. Si existen
-- servicios 'job' vivos la reversión falla por el CHECK; eliminarlos antes.

ALTER TABLE managed_services DROP CONSTRAINT IF EXISTS managed_services_kind_check;
ALTER TABLE managed_services ADD CONSTRAINT managed_services_kind_check
    CHECK (kind IN ('postgres', 'storage', 'functions', 'cron', 'auth'));
