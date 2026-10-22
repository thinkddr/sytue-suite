-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Wave 10 — motor de facturación (sobre el metering de la Wave 4).
--
-- 1) billing_accounts: una fila por tenant (PK = tenant). Guarda el plan de
--    cobro, un tope de gasto OPCIONAL (spend_limit_eur NULL = sin tope) y el id
--    de customer de Stripe (NULL = facturación local, sin pasarela). El plan por
--    defecto es 'free'. GetOrCreateBillingAccount hace upsert idempotente.
-- 2) invoices: artefacto read-mostly por periodo cerrado. UNIQUE(tenant,
--    period_from, period_to) da IDEMPOTENCIA a la generación: re-ejecutar el
--    mismo periodo no duplica (ON CONFLICT DO NOTHING + SELECT existente). Las
--    líneas van en jsonb (metric, quantity, unit_price_eur, subtotal_eur): un
--    artefacto que se lee entero, sin joins ni consultas por-línea → evita una
--    tabla invoice_lines que nunca se filtraría por sí sola (ponytail).
--    subtotal_eur = suma de overage de las líneas; total_eur = subtotal + base
--    del plan. status: draft|issued|paid|void.
-- 3) Índice keyset por (tenant, period_from DESC, id DESC) para ListInvoices sin
--    OFFSET a escala.
--
-- RLS deny-by-default por tenant en AMBAS tablas (patrón 0001/0013): FORCE ROW
-- LEVEL SECURITY + política que compara `tenant` con current_setting('app.tenant').
-- pgtest corre como superusuario y BYPASSEA RLS, así que la política se DECLARA
-- aquí (un test verde no la probaría); en prod con FORCE es obligatoria o el
-- tenant lee 0 filas.

CREATE TABLE billing_accounts (
    tenant             text        PRIMARY KEY,
    plan               text        NOT NULL DEFAULT 'free',
    spend_limit_eur    numeric,
    stripe_customer_id text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE invoices (
    id                uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant            text        NOT NULL,
    period_from       timestamptz NOT NULL,
    period_to         timestamptz NOT NULL,
    status            text        NOT NULL DEFAULT 'draft',
    subtotal_eur      numeric     NOT NULL,
    total_eur         numeric     NOT NULL,
    lines             jsonb       NOT NULL DEFAULT '[]'::jsonb,
    stripe_invoice_id text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    issued_at         timestamptz,
    UNIQUE (tenant, period_from, period_to)
);

CREATE INDEX invoices_tenant_period_idx ON invoices (tenant, period_from DESC, id DESC);

ALTER TABLE billing_accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE billing_accounts FORCE ROW LEVEL SECURITY;
ALTER TABLE invoices         ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoices         FORCE ROW LEVEL SECURITY;

CREATE POLICY billing_accounts_tenant_isolation ON billing_accounts
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY invoices_tenant_isolation ON invoices
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
