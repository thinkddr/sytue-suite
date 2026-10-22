-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Cola de denuncias del OPERADOR de la plataforma (ADR 0074, «Cola del operador»).
--
-- Las denuncias llegan a dos colas: la de quien administra la organización del
-- contenido y la de Sytue. Son las MISMAS filas, así que resolverla en una la
-- cierra en las dos; `resolved_from` dice desde cuál: 'org' (su administración)
-- o 'platform' (el operador). Vacío en las resueltas antes de existir la columna,
-- que solo podían ser de 'org'.
ALTER TABLE reports
    ADD COLUMN resolved_from text NOT NULL DEFAULT ''
        CHECK (resolved_from IN ('', 'org', 'platform'));

-- La cola del operador es de todas las organizaciones, y la purga también: las
-- dos van por la política de mantenimiento. Una transacción de negocio no la fija.
CREATE POLICY reports_maintenance ON reports
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
