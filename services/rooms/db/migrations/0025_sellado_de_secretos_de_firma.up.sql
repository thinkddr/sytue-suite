-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Deja sellar los secretos de firma HEREDADOS, los que se escribieron en claro
-- antes de que Rooms cifrara en reposo.
--
-- La lectura es tolerante a propósito (keycrypto devuelve tal cual lo que no
-- lleva el prefijo `encv1:`) para que una integración creada ayer siga firmando
-- hoy. Esa tolerancia mantiene el servicio en pie, pero deja el secreto legible
-- para quien lea la base: hay que sellarlos, y sellarlos exige recorrer TODAS las
-- organizaciones de una pasada.
--
-- El repo ya tiene ese camino: la política PERMISSIVE `app.maintenance`, que solo
-- activa `InMaintenanceTx` y que ya usan `bots`, `incoming_webhooks` y
-- `scheduled_messages`. Estas dos tablas no la tenían porque hasta ahora nada
-- suyo cruzaba organizaciones.
--
-- No relaja el aislamiento: las políticas se combinan con OR y la de tenant sigue
-- intacta, así que una consulta normal —sin ese GUC— ve exactamente lo que veía.

CREATE POLICY outgoing_webhooks_maintenance ON outgoing_webhooks
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

CREATE POLICY slash_commands_maintenance ON slash_commands
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
