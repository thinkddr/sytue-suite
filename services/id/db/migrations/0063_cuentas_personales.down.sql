-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira las cuentas personales.
-- sytue:destructive-ok deshace 0063: las columnas `kind` y `plan` solo las escribe
-- el binario nuevo y las filas personales quedan como organizaciones corrientes
-- de una persona, que es exactamente lo que eran antes de esta migración.

DROP INDEX IF EXISTS organizations_personales_idx;

ALTER TABLE org_subscriptions
    DROP CONSTRAINT org_subscriptions_unit_price_cents_check;

UPDATE org_subscriptions SET unit_price_cents = 600 WHERE unit_price_cents IN (0, 500, 2000);

ALTER TABLE org_subscriptions
    ALTER COLUMN unit_price_cents SET DEFAULT 600;

ALTER TABLE org_subscriptions
    ADD CONSTRAINT org_subscriptions_unit_price_cents_check
        CHECK (unit_price_cents = 600);

ALTER TABLE org_subscriptions DROP COLUMN plan;
ALTER TABLE organizations DROP COLUMN kind;
