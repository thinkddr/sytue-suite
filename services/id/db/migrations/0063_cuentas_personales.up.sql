-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Cuentas personales: registro gratuito con 5 GB y plan de 1 TB por 5 EUR al mes.
--
-- Una cuenta personal ES una organización —el modelo de datos sigue siendo
-- único, como ya decidió el alta guiada (mig 0027)—, pero con dos diferencias
-- que hay que poder consultar:
--
--   * `organizations.kind`: 'org' (la de siempre) o 'personal'. Una personal
--     tiene una sola persona, su nombre es su correo y NO da acceso a Sytue Admin
--     ni a SyERP. Esa verja no puede apoyarse en el rol —la persona es owner de
--     su propia organización— así que necesita esta columna.
--
--   * `org_subscriptions.plan`: el contrato. La suite por asientos sigue siendo
--     'suite_one' y no cambia de precio; los personales son 'personal_free' y
--     'personal_1tb'. El precio unitario pasa a admitir 0 (gratis) y 500
--     (5 EUR): la CHECK se redefine con el mismo nombre, que es como este repo
--     amplía una constraint sin que el gate de migraciones lo tome por retirada.
--
-- Expand puro: columnas nuevas con DEFAULT, ninguna fila existente cambia de
-- significado y el binario anterior sigue leyendo lo que leía.

ALTER TABLE organizations
    ADD COLUMN kind text NOT NULL DEFAULT 'org'
        CONSTRAINT organizations_kind_check CHECK (kind IN ('org', 'personal'));

ALTER TABLE org_subscriptions
    ADD COLUMN plan text NOT NULL DEFAULT 'standard'
        CONSTRAINT org_subscriptions_plan_check
        CHECK (plan IN ('personal_free', 'standard', 'pro'));

-- El asiento de 6 EUR deja de existir (ADR 0038, 2026-09-13): se suelta la
-- restricción anterior que exigía exactamente 600 céntimos antes de rebajarlo al
-- estándar de 5 EUR (500 céntimos). En el piloto no hay ningún cobro hecho.
ALTER TABLE org_subscriptions
    DROP CONSTRAINT org_subscriptions_unit_price_cents_check;

UPDATE org_subscriptions SET unit_price_cents = 500 WHERE unit_price_cents = 600;

ALTER TABLE org_subscriptions
    ALTER COLUMN unit_price_cents SET DEFAULT 500;

ALTER TABLE org_subscriptions
    ADD CONSTRAINT org_subscriptions_unit_price_cents_check
        CHECK (unit_price_cents IN (0, 500, 2000));

-- Buscar las cuentas personales sin recorrer las organizaciones de verdad, que
-- son pocas pero pesan: el índice es parcial porque solo se pregunta por ellas.
CREATE INDEX organizations_personales_idx ON organizations (created_at)
    WHERE kind = 'personal';
