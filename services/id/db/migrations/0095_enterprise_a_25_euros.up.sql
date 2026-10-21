-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Enterprise pasa de 20 a 25 EUR por usuario y mes (2026-09-22). El precio de una
-- fila lo fija el servidor desde commerce (EnsureSubscriptionPlan y el checkout),
-- así que sin 2500 en el CHECK fallaría CUALQUIER alta de un contrato Enterprise,
-- se cobre o no. 2000 se queda: puede haber filas pendientes con el precio de antes.

ALTER TABLE org_subscriptions DROP CONSTRAINT IF EXISTS org_subscriptions_unit_price_cents_check;
ALTER TABLE org_subscriptions
    ADD CONSTRAINT org_subscriptions_unit_price_cents_check
        CHECK (unit_price_cents IN (0, 250, 500, 2000, 2500));
