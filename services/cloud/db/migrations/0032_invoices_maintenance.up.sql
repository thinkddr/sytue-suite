-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Política PERMISSIVE de mantenimiento sobre `invoices` (patrón 0002/0003/0016).
--
-- La necesita el emisor mensual de facturas: para saber a QUÉ tenants les falta
-- la factura del periodo cerrado hace una consulta CROSS-TENANT sobre
-- billing_accounts (que ya abrió 0016) con un NOT EXISTS contra invoices. Con
-- FORCE ROW LEVEL SECURITY y sin esta política ese subselect ve CERO filas
-- siempre, así que el barrido creería que a todo el mundo le falta la factura y
-- volvería a agregar el uso de cada tenant en cada ciclo, para siempre: no
-- emitiría facturas de más —CreateInvoice es idempotente por el UNIQUE de
-- periodo— pero nunca convergería.
--
-- Es USING (lectura) únicamente: emitir la factura sigue haciéndose DENTRO del
-- contexto del tenant, gobernado por la política de aislamiento de 0015 (que es
-- la que trae el WITH CHECK). Las transacciones de negocio nunca fijan
-- app.maintenance, así que el aislamiento por tenant queda intacto.

CREATE POLICY invoices_maintenance ON invoices
    USING (current_setting('app.maintenance', true) = 'on');
