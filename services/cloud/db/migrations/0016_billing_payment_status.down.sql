-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte 0016_billing_payment_status.

DROP POLICY IF EXISTS billing_accounts_maintenance ON billing_accounts;
ALTER TABLE billing_accounts DROP COLUMN IF EXISTS payment_status;
