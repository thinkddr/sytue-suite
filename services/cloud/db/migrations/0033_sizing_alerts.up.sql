-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Avisos de tamaño ya ENTREGADOS, para no repetirlos.
--
-- La recomendación de tamaño (0026 + 0029) solo se veía ENTRANDO al proyecto, así
-- que el ahorro dependía de que alguien abriera el panel. La promesa del producto
-- es la contraria: avisar nosotros. Esta tabla es lo que permite avisar sin
-- convertirse en ruido.
--
-- Una fila POR PROYECTO —no por aviso— y guarda la ÚLTIMA recomendación avisada:
--
--  1. Repetir la MISMA recomendación es insistir, y quien no la aplicó ya la tiene
--     en el panel. Por eso el UPDATE lleva `IS DISTINCT FROM`: mientras el consejo
--     no cambie, no se vuelve a escribir ni a emitir nada.
--  2. La exclusión mutua entre réplicas la da la CLAVE PRIMARIA, igual que en
--     billing_spend_alerts (0022): el ON CONFLICT es atómico, así que de N
--     evaluadores simultáneos exactamente uno emite el evento.
--  3. notified_at sostiene el enfriamiento: una recomendación que oscila
--     (down → up → down) avisaría en cada ciclo, y esa es exactamente la forma de
--     que el cliente silencie el canal y se pierda el aviso que sí importaba.
CREATE TABLE sizing_alerts (
    -- El proyecto es la clave: un solo aviso vivo por proyecto.
    project_id  uuid        NOT NULL PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    tenant      text        NOT NULL,
    -- Recomendación avisada: nombres del catálogo (domain.InstanceSizes).
    from_size   text        NOT NULL,
    to_size     text        NOT NULL,
    notified_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE sizing_alerts ENABLE ROW LEVEL SECURITY;
ALTER TABLE sizing_alerts FORCE ROW LEVEL SECURITY;

CREATE POLICY sizing_alerts_tenant ON sizing_alerts
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- El evaluador recorre TODOS los tenants, así que no puede ir bajo la política
-- por-tenant. Mismo criterio EXACTO que la 0022 y la 0003: un flag explícito
-- (`app.maintenance='on'`, que solo fija pg.InMaintenanceTx) y no «sin tenant
-- fijado», que abriría la tabla a cualquier transacción que se olvidara de fijarlo.
CREATE POLICY sizing_alerts_maintenance ON sizing_alerts
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

-- El CHECK de alert_policies (0010, ampliado en la 0022) enumera los eventos
-- permitidos: sin ampliarlo, una política de aviso de tamaño la RECHAZA la base de
-- datos y el evaluador emitiría eventos que nadie puede escuchar.
ALTER TABLE alert_policies DROP CONSTRAINT IF EXISTS alert_policies_event_check;
ALTER TABLE alert_policies ADD CONSTRAINT alert_policies_event_check
    CHECK (event IN ('deploy.failed', 'deploy.ready', 'uptime.down', 'uptime.up',
                     'billing.spend_threshold', 'sizing.advice'));
