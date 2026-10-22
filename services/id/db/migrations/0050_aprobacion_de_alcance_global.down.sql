-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revertir devuelve la concesión de alcance global a una sola firma.
--
-- Las solicitudes de esa acción se retiran ANTES de reponer el CHECK, y todas,
-- no solo las pendientes: con una sola fila que lo incumpla PostgreSQL no deja
-- crear la restricción, y la reversión se quedaría a medias dejando la tabla sin
-- protección ninguna. Un `down` que puede fallar a mitad no es un `down`.
--
-- Lo que se pierde es el rastro de quién pidió y quién firmó los alcances
-- concedidos por esta vía. La concesión en sí NO se toca: vive en
-- `platform_admins`, con su `granted_by` y su motivo, y el hecho sigue además en
-- la auditoría (`platform.admin.granted`), que es donde se va a mirar.
DELETE FROM platform_approvals WHERE action = 'platform.admin.grant';

ALTER TABLE platform_approvals
    DROP CONSTRAINT IF EXISTS platform_approvals_action_check;

ALTER TABLE platform_approvals
    ADD CONSTRAINT platform_approvals_action_check
    CHECK (action IN ('org.suspend', 'org.merge'));
