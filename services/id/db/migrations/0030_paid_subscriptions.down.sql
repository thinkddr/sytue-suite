-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP TRIGGER IF EXISTS subscription_payments_set_updated_at ON subscription_payments;
DROP TRIGGER IF EXISTS org_subscriptions_set_updated_at ON org_subscriptions;
DROP TABLE IF EXISTS subscription_payments;
DROP TABLE IF EXISTS org_subscriptions;
