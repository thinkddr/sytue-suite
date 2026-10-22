-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El derecho de acceso vive junto a identidad porque Sytue ID es la única puerta
-- común de web, escritorio y móvil. Ningún webhook puede escribir una fila
-- "active" sin que el backend haya vuelto a consultar el pago en Mollie.

CREATE TABLE org_subscriptions (
    org_id                    uuid PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    provider                  text NOT NULL DEFAULT 'mollie' CHECK (provider = 'mollie'),
    provider_customer_id      text NOT NULL DEFAULT '',
    provider_subscription_id  text NOT NULL DEFAULT '',
    status                    text NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('pending', 'active', 'past_due', 'canceled')),
    seats                     integer NOT NULL DEFAULT 1 CHECK (seats > 0 AND seats <= 10000),
    unit_price_cents          integer NOT NULL DEFAULT 600 CHECK (unit_price_cents = 600),
    currency                  text NOT NULL DEFAULT 'EUR' CHECK (currency = 'EUR'),
    paid_until                timestamptz,
    created_at                timestamptz NOT NULL DEFAULT now(),
    updated_at                timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX org_subscriptions_customer_idx
    ON org_subscriptions (provider_customer_id) WHERE provider_customer_id <> '';
CREATE UNIQUE INDEX org_subscriptions_provider_id_idx
    ON org_subscriptions (provider_subscription_id) WHERE provider_subscription_id <> '';

CREATE TABLE subscription_payments (
    provider_payment_id text PRIMARY KEY,
    org_id               uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    status               text NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('open', 'pending', 'authorized', 'paid',
                                           'failed', 'canceled', 'expired',
                                           'refunded', 'charged_back')),
    amount_cents         integer NOT NULL CHECK (amount_cents > 0),
    currency             text NOT NULL DEFAULT 'EUR' CHECK (currency = 'EUR'),
    paid_at              timestamptz,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX subscription_payments_org_created_idx
    ON subscription_payments (org_id, created_at DESC);

CREATE TRIGGER org_subscriptions_set_updated_at BEFORE UPDATE ON org_subscriptions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER subscription_payments_set_updated_at BEFORE UPDATE ON subscription_payments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- La migración no concede acceso retroactivamente. Las organizaciones existentes
-- quedan pendientes hasta completar un pago real de 6 EUR por asiento.
INSERT INTO org_subscriptions (org_id, seats)
SELECT o.id, greatest(1, count(u.id)::integer)
FROM organizations o
LEFT JOIN memberships m ON m.org_id = o.id
LEFT JOIN users u ON u.id = m.user_id AND u.status = 'active'
GROUP BY o.id
ON CONFLICT (org_id) DO NOTHING;
