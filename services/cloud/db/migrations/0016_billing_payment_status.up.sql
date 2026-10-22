-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Estado de pago del tenant, gobernado por el webhook de Stripe (Gap 2).
--
-- 1) payment_status: 'active' por defecto (facturación local o al día). El
--    webhook de Stripe lo transiciona a 'past_due' (invoice.payment_failed) o
--    'canceled' (customer.subscription.deleted), y de vuelta a 'active' cuando
--    se cobra (invoice.paid/payment_succeeded). CHECK acota los valores.
-- 2) Política PERMISSIVE de mantenimiento (patrón 0002/0003): el webhook
--    POST /webhooks/stripe es PÚBLICO (sin tenant en el contexto) y debe resolver
--    customer→tenant leyendo billing_accounts CROSS-TENANT vía pg.InMaintenanceTx.
--    Con FORCE ROW LEVEL SECURITY eso es deny-by-default (0 filas) salvo esta
--    política, que solo abre lectura cuando la sesión fija app.maintenance='on'.
--    Es USING (SELECT) únicamente: el UPDATE del estado se hace DESPUÉS en el
--    contexto del tenant ya resuelto, gobernado por la política de aislamiento
--    por-tenant de 0015 (WITH CHECK). Las transacciones de negocio nunca fijan
--    app.maintenance, así que el aislamiento por tenant queda intacto.

ALTER TABLE billing_accounts
    ADD COLUMN payment_status text NOT NULL DEFAULT 'active'
        CHECK (payment_status IN ('active', 'past_due', 'canceled'));

CREATE POLICY billing_accounts_maintenance ON billing_accounts
    USING (current_setting('app.maintenance', true) = 'on');
