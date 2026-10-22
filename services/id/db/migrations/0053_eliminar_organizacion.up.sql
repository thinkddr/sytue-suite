-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Eliminar una organización pasa a exigir DOS FIRMAS, como suspenderla y como
-- fusionarla.
--
-- El estado `deleted` existía desde la primera migración —el CHECK de
-- `organizations.status` ya lo admite— y el dominio tenía su `StatusDeleted`,
-- pero no había forma de pedirlo: `handlePlatformSetOrgStatus` solo aceptaba
-- `active` y `suspended`. Una capacidad modelada de punta a punta y sin puerta.
--
-- Lo que falta para abrirla es esto: el CHECK de `platform_approvals.action` es
-- una lista cerrada, así que una solicitud `org.delete` se rechazaba en la base
-- ANTES de llegar a ninguna parte. Sin esta migración, el botón nuevo devolvería
-- un 500 sin explicación.
--
-- Y ELIMINAR NO ES `DELETE FROM`. `organizations` solo tiene dos claves foráneas
-- con `ON DELETE CASCADE` apuntándole; el resto del esquema se aísla por RLS y
-- por columna `org_id`, sin FK. Un borrado físico dejaría huérfanas las
-- membresías, los buzones, los grupos, los roles y el registro de auditoría, que
-- es precisamente lo que hay que conservar cuando una empresa se va. El estado
-- deja la fila para poder responder qué fue de ella, y permite deshacerlo.

ALTER TABLE platform_approvals
    DROP CONSTRAINT platform_approvals_action_check;

ALTER TABLE platform_approvals
    ADD CONSTRAINT platform_approvals_action_check
    CHECK (action IN ('org.suspend', 'org.merge', 'platform.admin.grant', 'org.delete'));
