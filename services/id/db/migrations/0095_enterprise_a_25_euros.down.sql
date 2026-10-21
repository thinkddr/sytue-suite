-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Vuelta al CHECK de la 0064. Se NIEGA si hay contratos a 25 EUR: reescribir su
-- importe dejaría la fila diciendo un precio distinto del que se firmó, y eso no
-- se arregla bajando una migración sino a mano, sabiendo a quién se le cobra qué.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM org_subscriptions WHERE unit_price_cents = 2500) THEN
        RAISE EXCEPTION 'hay contratos Enterprise a 25 EUR: no se puede bajar la 0095 sin revisarlos a mano';
    END IF;
END $$;
ALTER TABLE org_subscriptions DROP CONSTRAINT IF EXISTS org_subscriptions_unit_price_cents_check;
ALTER TABLE org_subscriptions
    ADD CONSTRAINT org_subscriptions_unit_price_cents_check
        CHECK (unit_price_cents IN (0, 250, 500, 2000));
