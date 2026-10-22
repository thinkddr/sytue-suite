-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Avisos de consumo al límite (CPU y memoria), ya entregados, para no repetirlos.
--
-- El comentario de domain.go daba estas alertas por imposibles «hasta que exista
-- el stack de métricas vivo». Existe desde la 0026: `resource_samples` guarda una
-- muestra cada 15 minutos con 30 días de retención, y el motor de rightsizing ya
-- calcula sobre ella el p95 de memoria y de CPU. O sea: la señal estaba medida y
-- nadie la escuchaba. Es la alerta de «CPU/Memory utilization» que DigitalOcean
-- ofrece de serie, y la única que avisa ANTES del OOMKill.
--
-- Una fila por (proyecto, recurso), no por aviso:
--
--  1. La exclusión mutua entre réplicas la da la CLAVE PRIMARIA, igual que en
--     sizing_alerts (0033) y billing_spend_alerts (0022): el ON CONFLICT es
--     atómico, así que de N evaluadores simultáneos exactamente uno emite.
--  2. notified_at sostiene el enfriamiento. Un consumo que baila alrededor del
--     umbral avisaría en cada ciclo, que es la forma más rápida de que el cliente
--     silencie el canal y se pierda el aviso que sí importaba.
--  3. Por RECURSO y no solo por proyecto: quedarse sin memoria y quedarse sin CPU
--     son dos problemas con dos arreglos distintos, y uno no puede tapar al otro.
CREATE TABLE metric_alerts (
    project_id  uuid        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    -- 'cpu' | 'memory'. Sin CHECK enumerado a propósito: el conjunto lo fija el
    -- dominio (domain.MetricKind) y aquí una lista más sería otra copia que
    -- desincronizar. Lo que la base garantiza es la unicidad, que es lo que
    -- sostiene la exclusión mutua.
    kind        text        NOT NULL,
    tenant      text        NOT NULL,
    -- Porcentaje del límite que se avisó: permite no repetir hasta que empeore.
    percent     int         NOT NULL,
    notified_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, kind)
);

ALTER TABLE metric_alerts ENABLE ROW LEVEL SECURITY;
ALTER TABLE metric_alerts FORCE ROW LEVEL SECURITY;

CREATE POLICY metric_alerts_tenant ON metric_alerts
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- El evaluador recorre TODOS los tenants, así que no puede ir bajo la política
-- por-tenant. Mismo criterio EXACTO que la 0033, la 0022 y la 0003: un flag
-- explícito (`app.maintenance='on'`, que solo fija pg.InMaintenanceTx) y no «sin
-- tenant fijado», que abriría la tabla a cualquier transacción despistada.
CREATE POLICY metric_alerts_maintenance ON metric_alerts
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

-- El CHECK de alert_policies (0010, ampliado en la 0022 y la 0033) enumera los
-- eventos permitidos: sin ampliarlo, una política de aviso de consumo la RECHAZA
-- la base de datos y el evaluador emitiría eventos que nadie puede escuchar.
ALTER TABLE alert_policies DROP CONSTRAINT IF EXISTS alert_policies_event_check;
ALTER TABLE alert_policies ADD CONSTRAINT alert_policies_event_check
    CHECK (event IN ('deploy.failed', 'deploy.ready', 'uptime.down', 'uptime.up',
                     'billing.spend_threshold', 'sizing.advice',
                     'metric.cpu_high', 'metric.memory_high'));

-- Política de FUSIÓN de organizaciones. La 0028 se la dio a todas las tablas con
-- columna `tenant` que existían entonces, y cada tabla nueva tiene que traer la
-- suya: bajo FORCE ROW LEVEL SECURITY el motor de fusión no vería estas filas y
-- el preflight abortaría la fusión ENTERA señalando una tabla que nadie
-- relaciona con la migración que la creó. Le pasó a `sizing_alerts` (0033), que
-- necesitó la 0034 para arreglarlo; aquí va desde el principio.
--
-- Con el nombre escrito y no con un barrido dinámico: el gate que lo caza
-- (`platform/pkg/migcheck`) lee el SQL como TEXTO, así que un `EXECUTE
-- format(...)` le resultaría invisible.
DROP POLICY IF EXISTS metric_alerts_platform_merge ON metric_alerts;

CREATE POLICY metric_alerts_platform_merge ON metric_alerts
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
