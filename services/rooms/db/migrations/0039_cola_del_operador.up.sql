-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Cola de denuncias del OPERADOR de la plataforma (ADR 0073, «Cola del operador»).
--
-- Las denuncias llegan a dos colas: la de quien modera en la organización que
-- aloja la conversación y la de Sytue. Son las MISMAS filas, así que resolverla en
-- una la cierra en las dos; `resolved_from` dice desde cuál: 'org' o 'platform'.
-- Vacío en las resueltas antes de existir la columna, que solo podían ser de 'org'.
ALTER TABLE message_reports
    ADD COLUMN resolved_from text NOT NULL DEFAULT ''
        CHECK (resolved_from IN ('', 'org', 'platform'));

-- La cola del operador es de todas las organizaciones, y la purga también: las
-- dos encuentran las filas por la política de mantenimiento. El canal y el
-- mensaje se leen después en la transacción de SU organización (store/operador.go).
CREATE POLICY message_reports_maintenance ON message_reports
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
