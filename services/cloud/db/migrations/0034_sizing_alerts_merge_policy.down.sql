-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira la política de fusión de `sizing_alerts`. El aislamiento por tenant no
-- depende de ella: lo aplica `sizing_alerts_tenant`, que sigue en pie.

DROP POLICY IF EXISTS sizing_alerts_platform_merge ON sizing_alerts;
