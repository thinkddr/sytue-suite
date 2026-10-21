-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Sin los workers no puede quedar ninguno, o el ALTER falla.
DELETE FROM managed_services WHERE kind = 'worker';
ALTER TABLE managed_services DROP CONSTRAINT IF EXISTS managed_services_kind_check;
ALTER TABLE managed_services ADD CONSTRAINT managed_services_kind_check
    CHECK (kind IN ('postgres', 'storage', 'functions', 'cron', 'job', 'auth', 'app'));
