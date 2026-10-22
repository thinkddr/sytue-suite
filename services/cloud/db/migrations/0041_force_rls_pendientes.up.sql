-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- FORCE, no solo ENABLE.
--
-- `ENABLE ROW LEVEL SECURITY` no se aplica al DUEÑO de la tabla: el rol
-- propietario sigue viendo y escribiendo todas las filas de todos los tenants
-- como si no hubiera política. `FORCE` es lo que se la aplica también a él.
--
-- Estas tres se quedaron con ENABLE y sin FORCE mientras el resto del esquema
-- llevaba las dos (auditoría 2026-08-18: 112 tablas con FORCE, once sin él). No
-- es una diferencia de criterio, es un olvido repetido: el aislamiento
-- multi-tenant de estas dependía de que la aplicación no conectara nunca con el
-- rol propietario, en vez de estar garantizado por la base.
ALTER TABLE alert_policies   FORCE ROW LEVEL SECURITY;
ALTER TABLE alert_deliveries FORCE ROW LEVEL SECURITY;
ALTER TABLE uptime_checks    FORCE ROW LEVEL SECURITY;
