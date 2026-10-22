-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- organizations: política PERMISSIVE de mantenimiento (patrón 0002/0003/0004).
--
-- GetOrganizationPlanByTenant lee el plan de facturación CROSS-TENANT vía
-- pg.InMaintenanceTx (el reconciliador resuelve el plan de un tenant arbitrario).
-- organizations tiene FORCE ROW LEVEL SECURITY y, hasta ahora, SOLO la política
-- organizations_tenant_isolation (0001): con el rol real de la app (SIN BYPASSRLS)
-- y sin app.tenant fijado a ese tenant, la lectura devolvía 0 filas. Esta política
-- PERMISSIVE solo abre cuando la sesión fija app.maintenance='on' (pg.InMaintenanceTx);
-- las transacciones de negocio nunca lo fijan, así que el aislamiento por tenant
-- queda intacto. USING sin WITH CHECK: solo lectura cross-tenant (no hay INSERT/UPDATE
-- de organizations en mantenimiento).
CREATE POLICY organizations_maintenance ON organizations
    USING (current_setting('app.maintenance', true) = 'on');
