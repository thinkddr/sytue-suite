-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sytue Pages, tienda y formularios (PAGES-ENCARGO §10).
-- Catálogo, variantes, historial de precios, pasarelas con credenciales selladas,
-- pedidos, pagos, reembolsos, desistimientos y formularios.

-- ---------------------------------------------------------------------------
-- Ajustes de tienda
-- ---------------------------------------------------------------------------
CREATE TABLE store_settings (
    site_id           uuid PRIMARY KEY REFERENCES sites(id) ON DELETE CASCADE,
    tenant            text NOT NULL,
    enabled           boolean NOT NULL DEFAULT false,
    currency          text NOT NULL DEFAULT 'EUR' CHECK (currency = 'EUR'),
    countries         text[] NOT NULL DEFAULT '{ES}',
    shipping          jsonb NOT NULL DEFAULT '[]'::jsonb,
    terms             jsonb NOT NULL DEFAULT '{}'::jsonb,
    terms_version     integer NOT NULL DEFAULT 1,
    notify            jsonb NOT NULL DEFAULT '[]'::jsonb,
    reply_to          text NOT NULL DEFAULT '',
    erp_enabled       boolean NOT NULL DEFAULT false,
    next_order_number integer NOT NULL DEFAULT 1,
    updated_at        timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Productos y variantes
-- ---------------------------------------------------------------------------
CREATE TABLE products (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant             text NOT NULL,
    site_id            uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    slug               jsonb NOT NULL,
    name               jsonb NOT NULL,
    description        jsonb NOT NULL DEFAULT '{}'::jsonb,
    category           text NOT NULL DEFAULT '' CHECK (length(category) <= 60),
    images             uuid[] NOT NULL DEFAULT '{}',
    vat_rate           numeric(5,2) NOT NULL CHECK (vat_rate IN (0, 4, 5, 10, 21)),
    options            jsonb NOT NULL DEFAULT '[]'::jsonb,
    kind               text NOT NULL DEFAULT 'general' CHECK (kind IN ('general', 'food')),
    gpsr               jsonb NOT NULL DEFAULT '{}'::jsonb,
    food               jsonb NOT NULL DEFAULT '{}'::jsonb,
    withdrawal_exempt  text NOT NULL DEFAULT '' CHECK (withdrawal_exempt IN ('', 'perishable', 'custom', 'sealed')),
    max_per_order      integer NOT NULL DEFAULT 10 CHECK (max_per_order BETWEEN 1 AND 99),
    active             boolean NOT NULL DEFAULT true,
    position           integer NOT NULL DEFAULT 0,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX products_por_sitio ON products (tenant, site_id, position);

CREATE TABLE variants (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant           text NOT NULL,
    site_id          uuid NOT NULL,
    product_id       uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    sku              text NOT NULL CHECK (sku ~ '^[A-Za-z0-9._-]{1,64}$'),
    options          jsonb NOT NULL DEFAULT '{}'::jsonb,
    price_cents      integer NOT NULL CHECK (price_cents > 0),
    compare_at_cents integer CHECK (compare_at_cents IS NULL OR compare_at_cents > price_cents),
    stock            integer CHECK (stock IS NULL OR stock >= 0),
    reserved         integer NOT NULL DEFAULT 0 CHECK (reserved >= 0),
    CHECK (stock IS NULL OR reserved <= stock),
    UNIQUE (site_id, sku)
);
CREATE INDEX variants_por_producto ON variants (tenant, product_id);

CREATE TABLE price_history (
    variant_id  uuid NOT NULL REFERENCES variants(id) ON DELETE CASCADE,
    tenant      text NOT NULL,
    price_cents integer NOT NULL,
    since       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (variant_id, since)
);

-- ---------------------------------------------------------------------------
-- Pasarelas de pago
-- ---------------------------------------------------------------------------
CREATE TABLE gateways (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant             text NOT NULL,
    site_id            uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    provider           text NOT NULL CHECK (provider IN ('mollie', 'adyen', 'redsys', 'stripe')),
    mode               text NOT NULL CHECK (mode IN ('test', 'live')),
    credentials_enc    text NOT NULL CHECK (credentials_enc LIKE 'encv1:%'),
    public_config      jsonb NOT NULL DEFAULT '{}'::jsonb,
    next_ref           bigint NOT NULL DEFAULT 1,
    active             boolean NOT NULL DEFAULT true,
    notice_accepted_at timestamptz,
    notice_accepted_by text NOT NULL DEFAULT '',
    checked_at         timestamptz NOT NULL,
    created_by         text NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    CHECK (provider <> 'stripe' OR notice_accepted_at IS NOT NULL),
    UNIQUE (site_id, provider)
);

-- ---------------------------------------------------------------------------
-- Pedidos y líneas
-- ---------------------------------------------------------------------------
CREATE TABLE orders (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant              text NOT NULL,
    site_id             uuid NOT NULL REFERENCES sites(id) ON DELETE RESTRICT,
    number              text NOT NULL,
    test                boolean NOT NULL,
    checkout_token      uuid NOT NULL,
    access_token_hash   bytea NOT NULL,
    status              text NOT NULL CHECK (status IN ('pending_payment', 'paid', 'fulfilled', 'canceled', 'expired', 'refunded', 'partially_refunded', 'disputed')),
    oversold            boolean NOT NULL DEFAULT false,
    currency            text NOT NULL DEFAULT 'EUR',
    subtotal_cents      integer NOT NULL,
    shipping_cents      integer NOT NULL,
    total_cents         integer NOT NULL,
    vat_cents           integer NOT NULL,
    vat_breakdown       jsonb NOT NULL,
    buyer_email         text NOT NULL,
    buyer_name          text NOT NULL,
    buyer_phone         text NOT NULL DEFAULT '',
    shipping_method     jsonb NOT NULL,
    shipping_address    jsonb,
    invoice             jsonb,
    language            text NOT NULL,
    terms_version       integer NOT NULL,
    terms_accepted_at   timestamptz NOT NULL,
    reserved_until      timestamptz NOT NULL,
    paid_at             timestamptz,
    fulfilled_at        timestamptz,
    buyer_email_sent_at timestamptz,
    erp_status          text NOT NULL DEFAULT 'none' CHECK (erp_status IN ('none', 'pending', 'done', 'skipped', 'failed')),
    erp_order_ref       text NOT NULL DEFAULT '',
    erp_invoice_ref     text NOT NULL DEFAULT '',
    anonymized_at       timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (site_id, number),
    UNIQUE (site_id, checkout_token)
);
CREATE INDEX orders_pendientes ON orders (tenant, site_id) WHERE status = 'pending_payment';
CREATE INDEX orders_por_sitio ON orders (tenant, site_id, created_at DESC);

CREATE TABLE order_lines (
    order_id          uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    tenant            text NOT NULL,
    position          smallint NOT NULL,
    variant_id        uuid,
    sku               text NOT NULL,
    name              text NOT NULL,
    options           jsonb NOT NULL DEFAULT '{}'::jsonb,
    quantity          integer NOT NULL CHECK (quantity BETWEEN 1 AND 99),
    unit_price_cents  integer NOT NULL,
    vat_rate          numeric(5,2) NOT NULL,
    total_cents       integer NOT NULL,
    withdrawal_exempt text NOT NULL DEFAULT '',
    PRIMARY KEY (order_id, position)
);

-- ---------------------------------------------------------------------------
-- Pagos, eventos y reembolsos
-- ---------------------------------------------------------------------------
CREATE TABLE payments (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant               text NOT NULL,
    order_id             uuid NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    gateway_id           uuid NOT NULL REFERENCES gateways(id),
    attempt              smallint NOT NULL CHECK (attempt BETWEEN 1 AND 3),
    provider             text NOT NULL,
    method               text NOT NULL DEFAULT '',
    provider_ref         text NOT NULL DEFAULT '',
    status               text NOT NULL CHECK (status IN ('created', 'open', 'pending', 'authorized', 'paid', 'canceled', 'expired', 'failed', 'refunded', 'partially_refunded', 'charged_back')),
    amount_cents         integer NOT NULL,
    refunded_cents       integer NOT NULL DEFAULT 0,
    charged_back_cents   integer NOT NULL DEFAULT 0,
    idempotency_key      text NOT NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (order_id, attempt)
);
CREATE UNIQUE INDEX payments_ref ON payments (gateway_id, provider_ref) WHERE provider_ref <> '';

CREATE TABLE payment_events (
    tenant      text NOT NULL,
    gateway_id  uuid NOT NULL REFERENCES gateways(id),
    provider    text NOT NULL,
    event_id    text NOT NULL,
    payment_id  uuid REFERENCES payments(id),
    status      text NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (gateway_id, event_id)
);

CREATE TABLE refunds (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text NOT NULL,
    payment_id      uuid NOT NULL REFERENCES payments(id) ON DELETE RESTRICT,
    amount_cents    integer NOT NULL CHECK (amount_cents > 0),
    idempotency_key text NOT NULL UNIQUE,
    status          text NOT NULL CHECK (status IN ('pending', 'done', 'failed')),
    created_by      text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    done_at         timestamptz
);

CREATE TABLE withdrawals (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text NOT NULL,
    order_id        uuid NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    email           text NOT NULL,
    name            text NOT NULL DEFAULT '',
    source          text NOT NULL CHECK (source IN ('form', 'token')),
    reason          text NOT NULL DEFAULT '' CHECK (length(reason) <= 2000),
    created_at      timestamptz NOT NULL DEFAULT now(),
    ack_sent_at     timestamptz,
    acknowledged_at timestamptz
);

-- ---------------------------------------------------------------------------
-- Formularios
-- ---------------------------------------------------------------------------
CREATE TABLE form_settings (
    site_id  uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    tenant   text NOT NULL,
    form_id  text NOT NULL CHECK (form_id ~ '^[a-z0-9]{10}$'),
    enabled  boolean NOT NULL DEFAULT true,
    notify   jsonb NOT NULL DEFAULT '[]'::jsonb,
    PRIMARY KEY (site_id, form_id)
);

CREATE TABLE form_submissions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant       text NOT NULL,
    site_id      uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    form_id      text NOT NULL,
    data         jsonb NOT NULL,
    language     text NOT NULL,
    consent_text text NOT NULL,
    consent_at   timestamptz NOT NULL,
    read_at      timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX form_submissions_por_sitio ON form_submissions (tenant, site_id, form_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- RLS y políticas
-- ---------------------------------------------------------------------------
DO $$
DECLARE
    tbl text;
BEGIN
    FOR tbl IN SELECT unnest(ARRAY[
        'store_settings', 'products', 'variants', 'price_history', 'gateways',
        'orders', 'order_lines', 'payments', 'payment_events', 'refunds',
        'withdrawals', 'form_settings', 'form_submissions'
    ]) LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', tbl);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', tbl);

        EXECUTE format(
            'CREATE POLICY %I_tenant_isolation ON %I ' ||
            'USING (tenant = current_setting(''app.tenant'', true)) ' ||
            'WITH CHECK (tenant = current_setting(''app.tenant'', true))',
            tbl, tbl
        );

        EXECUTE format(
            'CREATE POLICY %I_platform_merge ON %I ' ||
            'USING (current_setting(''app.merge'', true) = ''on'') ' ||
            'WITH CHECK (current_setting(''app.merge'', true) = ''on'')',
            tbl, tbl
        );

        EXECUTE format(
            'CREATE POLICY %I_mantenimiento ON %I ' ||
            'USING (current_setting(''app.maintenance'', true) = ''on'') ' ||
            'WITH CHECK (current_setting(''app.maintenance'', true) = ''on'')',
            tbl, tbl
        );
    END LOOP;
END $$;
