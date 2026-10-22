-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Reconciliador cross-tenant: RLS de mantenimiento + teardown-una-vez + índices.
--
-- 1) Políticas PERMISSIVE de mantenimiento (patrón 0002) para las tablas que el
--    reconciliador del controller lee cross-tenant vía pg.InMaintenanceTx:
--    projects (JOIN de ListPendingDeployments/ListReadyProductions),
--    managed_services (ListPendingServices) y domains (ListPendingDomains).
--    Sin ellas, con FORCE RLS y un rol sin BYPASSRLS es deny-by-default (0 filas)
--    y el reconciliador nunca ve nada. Solo abren cuando la sesión fija
--    app.maintenance='on'; las transacciones de negocio nunca lo fijan, así que
--    el aislamiento por tenant queda intacto.
-- 2) torn_down_at: marca de retirada única de reconcileTeardown. NULL = objetos
--    aún por desmontar; se fija tras RetireApp. Desacopla el teardown del LIMIT y
--    de la ventana de recencia: los terminados en ráfaga (>LIMIT en una ventana)
--    ya no caducan sin desmontarse.
-- 3) Índices parciales que casan los predicados de las dos consultas nuevas del
--    reconciliador (Index Scan, no Seq Scan cada ciclo a escala).

-- 1) Mantenimiento ------------------------------------------------------------
CREATE POLICY projects_maintenance ON projects
    USING (current_setting('app.maintenance', true) = 'on');
CREATE POLICY managed_services_maintenance ON managed_services
    USING (current_setting('app.maintenance', true) = 'on');
CREATE POLICY domains_maintenance ON domains
    USING (current_setting('app.maintenance', true) = 'on');

-- 2) Teardown una vez ---------------------------------------------------------
ALTER TABLE deployments ADD COLUMN torn_down_at timestamptz;

-- 3) Índices ------------------------------------------------------------------
-- Teardown: terminados aún sin desmontar (backlog acotado), FIFO por updated_at.
CREATE INDEX deployments_teardown_idx ON deployments (updated_at)
    WHERE status IN ('canceled', 'retired') AND torn_down_at IS NULL;
-- Producción deseada: producciones ready por recencia; sirve el ORDER BY
-- updated_at DESC (reconcileProduction dedup por proyecto) sin sort.
CREATE INDEX deployments_ready_prod_idx ON deployments (updated_at DESC)
    WHERE target = 'production' AND status = 'ready';
