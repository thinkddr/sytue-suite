-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Avisos de coste ya ENTREGADOS, para no repetirlos.
--
-- El formulario del límite de gasto prometía «Te avisaremos al acercarte a X €» y
-- no había nada detrás. El evaluador corre en TODAS las réplicas de cloud-api, así
-- que hace falta que solo una de ellas avise: esta tabla es esa exclusión mutua.
--
-- No se usa un lock ni FOR UPDATE SKIP LOCKED: la clave primaria YA es la garantía.
-- Un INSERT ... ON CONFLICT DO NOTHING es atómico y sin bloqueos — la réplica cuyo
-- INSERT inserta fila es la que envía; las demás ven el conflicto y se callan.
--
-- El dedupe se ancla al PERIODO (mes natural, el mismo que factura el motor de
-- facturas). Al entrar el mes siguiente cambia `period_start`, así que los avisos
-- vuelven a estar disponibles solos: no hace falta ninguna tarea de limpieza ni
-- reinicio manual. Las filas viejas las barre la purga por antigüedad.

CREATE TABLE billing_spend_alerts (
    tenant        text        NOT NULL,
    -- Inicio del periodo de facturación (día 1 del mes, UTC).
    period_start  date        NOT NULL,
    -- Porcentaje del límite que se cruzó (80, 100…). Ver domain.SpendThresholds.
    threshold_pct integer     NOT NULL CHECK (threshold_pct > 0 AND threshold_pct <= 1000),
    -- Cifras del momento del aviso, para poder auditar por qué se envió.
    spend_eur     numeric     NOT NULL,
    limit_eur     numeric     NOT NULL,
    notified_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, period_start, threshold_pct)
);

-- RLS como el resto de tablas del tenant. El evaluador es un worker de fondo y
-- consulta cross-tenant con el rol de mantenimiento, igual que el prober de uptime.
ALTER TABLE billing_spend_alerts ENABLE ROW LEVEL SECURITY;
ALTER TABLE billing_spend_alerts FORCE ROW LEVEL SECURITY;

CREATE POLICY billing_spend_alerts_tenant ON billing_spend_alerts
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Política de mantenimiento: el evaluador recorre TODOS los tenants, así que no
-- puede ir bajo la política por-tenant. Mismo criterio EXACTO que la migración
-- 0003 (`app.maintenance='on'`, que solo fija pg.InMaintenanceTx): un flag
-- explícito y no «sin tenant fijado», que abriría la tabla a cualquier
-- transacción que se olvidara de fijarlo.
CREATE POLICY billing_spend_alerts_maintenance ON billing_spend_alerts
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

-- El CHECK de alert_policies (migración 0010) enumera los eventos permitidos, así
-- que sin ampliarlo una política de aviso de coste sería RECHAZADA por la BD y la
-- función quedaría muerta: el evaluador emitiría eventos que nadie podría escuchar.
ALTER TABLE alert_policies DROP CONSTRAINT IF EXISTS alert_policies_event_check;
ALTER TABLE alert_policies ADD CONSTRAINT alert_policies_event_check
    CHECK (event IN ('deploy.failed', 'deploy.ready', 'uptime.down', 'uptime.up',
                     'billing.spend_threshold'));
