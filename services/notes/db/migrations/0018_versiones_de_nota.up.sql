-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- HISTORIAL DE VERSIONES de una nota: cómo estaba antes de cada tanda de edición,
-- para verla y volver a ella.
--
--   · Se guarda lo que la nota tenía ANTES de un guardado que cambia su texto (título,
--     contenido o el cifrado), no lo nuevo: lo nuevo ya es la nota.
--   · En un cuaderno cifrado se guarda `content_cipher` tal cual: el servidor nunca ve
--     el texto en claro de una versión, igual que no ve el de la nota.
--   · Retención: las 50 últimas y nunca más de 30 días. La poda la hace el propio
--     guardado (store.guardarVersion) y la lectura no enseña lo caducado.
--   · Va en cascada con la nota: purgarla de la papelera se lleva su historial.
CREATE TABLE note_versions (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    note_id        uuid        NOT NULL REFERENCES notes (id) ON DELETE CASCADE,
    tenant         text        NOT NULL,
    replaced_by    text        NOT NULL, -- quien guardó encima
    title          text        NOT NULL DEFAULT '',
    content        text        NOT NULL DEFAULT '',
    content_cipher text        NOT NULL DEFAULT '',
    data           jsonb,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT note_version_cipher_len CHECK (octet_length(content_cipher) <= 16777216)
);

CREATE INDEX note_versions_nota ON note_versions (tenant, note_id, created_at DESC);

ALTER TABLE note_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE note_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY note_versions_tenant_isolation ON note_versions
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY note_versions_platform_merge ON note_versions
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
-- La purga de la papelera (0005) borra notas bajo `app.maintenance`: la cascada
-- necesita poder borrar también sus versiones.
CREATE POLICY note_versions_mantenimiento ON note_versions
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
