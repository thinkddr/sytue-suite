-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
ALTER TABLE organizations DROP COLUMN IF EXISTS education;
ALTER TABLE org_subscriptions DROP CONSTRAINT IF EXISTS org_subscriptions_unit_price_cents_check;
UPDATE org_subscriptions SET unit_price_cents = 500 WHERE unit_price_cents = 250;
ALTER TABLE org_subscriptions
    ADD CONSTRAINT org_subscriptions_unit_price_cents_check
        CHECK (unit_price_cents IN (0, 500, 2000));
ALTER TABLE org_subscriptions DROP CONSTRAINT IF EXISTS org_subscriptions_plan_check;
UPDATE org_subscriptions SET plan = 'standard' WHERE plan = 'pro';
UPDATE org_subscriptions SET plan = 'pro' WHERE plan = 'enterprise';
ALTER TABLE org_subscriptions ALTER COLUMN plan SET DEFAULT 'standard';
ALTER TABLE org_subscriptions
    ADD CONSTRAINT org_subscriptions_plan_check
        CHECK (plan IN ('personal_free', 'standard', 'pro'));
