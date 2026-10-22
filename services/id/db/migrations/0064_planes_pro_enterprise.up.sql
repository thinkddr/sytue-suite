-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Nombres finales de los planes (ADR 0038, 2026-09-14): `standard` pasa a
-- llamarse `pro` (5 EUR, 200 GB por usuario) y `pro` a `enterprise` (20 EUR,
-- 1 TB por usuario, Soporte Premium). El orden de los UPDATE importa: primero
-- se libera el nombre `pro`. Y las instituciones educativas tienen el Pro al
-- 50 % (organizations.education, lo concede la plataforma): el precio unitario
-- admite 250.

ALTER TABLE org_subscriptions DROP CONSTRAINT IF EXISTS org_subscriptions_plan_check;
UPDATE org_subscriptions SET plan = 'enterprise' WHERE plan = 'pro';
UPDATE org_subscriptions SET plan = 'pro' WHERE plan = 'standard';
ALTER TABLE org_subscriptions ALTER COLUMN plan SET DEFAULT 'pro';
ALTER TABLE org_subscriptions
    ADD CONSTRAINT org_subscriptions_plan_check
        CHECK (plan IN ('personal_free', 'pro', 'enterprise'));

ALTER TABLE org_subscriptions DROP CONSTRAINT IF EXISTS org_subscriptions_unit_price_cents_check;
ALTER TABLE org_subscriptions
    ADD CONSTRAINT org_subscriptions_unit_price_cents_check
        CHECK (unit_price_cents IN (0, 250, 500, 2000));

ALTER TABLE organizations
    ADD COLUMN education boolean NOT NULL DEFAULT false;
