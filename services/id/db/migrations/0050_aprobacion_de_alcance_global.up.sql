-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Conceder el alcance global también necesita dos personas.
--
-- La 0046 hizo colegiado lo irreversible: suspender una organización y
-- fusionarla. Se quedó fuera repartir el propio alcance de plataforma, y el
-- riesgo estaba justo al revés de como debería: suspender a UN cliente —que se
-- deshace reactivándolo— exigía dos firmas, y dar el permiso que permite
-- suspenderlos a TODOS, fusionar cualquier par de organizaciones y repartir ese
-- mismo permiso, bastaba con una. Una sola cuenta elevada y comprometida podía
-- fabricarse compañía permanente en un POST.
--
-- El aviso por correo a todos los admins (que ya existía) no cubre esto: avisa
-- DESPUÉS, y a un correo de madrugada se llega tarde.
--
-- Solo cambia el CHECK. `payload` ya es jsonb, así que la acción nueva guarda lo
-- suyo —el id de la cuenta y el motivo— sin tocar el esquema. Se guarda el id y
-- no el correo a propósito: entre pedir y firmar cabe un día, y quien firma
-- tiene que estar dando el alcance a la cuenta que se pidió, no a la que hoy
-- tenga ese correo.
--
-- No hay nada que migrar: las filas existentes son de las dos acciones que ya
-- eran válidas, así que el CHECK nuevo las acepta todas.
ALTER TABLE platform_approvals
    DROP CONSTRAINT platform_approvals_action_check;

ALTER TABLE platform_approvals
    ADD CONSTRAINT platform_approvals_action_check
    CHECK (action IN ('org.suspend', 'org.merge', 'platform.admin.grant'));
