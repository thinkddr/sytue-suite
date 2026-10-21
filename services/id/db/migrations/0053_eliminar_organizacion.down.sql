-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revertir quita la posibilidad de pedir la eliminación de una organización.
--
-- Las solicitudes de esa acción se retiran ANTES de reponer el CHECK, y TODAS,
-- no solo las pendientes: con una sola fila que lo incumpla PostgreSQL no deja
-- crear la restricción, y la reversión se quedaría a medias dejando la tabla sin
-- protección ninguna. Un `down` que puede fallar a mitad no es un `down`. Mismo
-- criterio que la 0050, por el mismo motivo.
--
-- Lo que se pierde es el rastro de quién pidió y quién firmó las eliminaciones.
-- El estado de las organizaciones NO se toca: una que quedara en `deleted` sigue
-- estándolo, porque `organizations.status` admite ese valor desde el principio y
-- reactivarla es un cambio de estado normal, no una migración.

DELETE FROM platform_approvals WHERE action = 'org.delete';

ALTER TABLE platform_approvals
    DROP CONSTRAINT platform_approvals_action_check;

ALTER TABLE platform_approvals
    ADD CONSTRAINT platform_approvals_action_check
    CHECK (action IN ('org.suspend', 'org.merge', 'platform.admin.grant'));
