-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Compartir un cuaderno o una nota con UNA persona de la organización, por su `sub`.
--
-- Un cuaderno se comparte en dos niveles, y cada uno vive en su tabla:
--   · leer   → `notebook_lectores` (esta migración);
--   · editar → `notebook_editores` (0009), la misma que concede «Pedir acceso de
--     edición». Así la escritura sigue saliendo de UNA tabla y las cláusulas de
--     escritura del store no cambian; lo que cambia es que tener fila en cualquiera
--     de las dos da de LEER un cuaderno que no es de la organización.
--
-- Una nota suelta se comparte en `note_personas`, con su nivel en la fila: quien la
-- recibe no ve el cuaderno, solo esa nota («Compartido conmigo»), y si puede editarla
-- cambia su contenido y nada más —ni la mueve, ni la borra, ni su visibilidad—.
CREATE TABLE notebook_lectores (
    notebook_id   uuid        NOT NULL REFERENCES notebooks (id) ON DELETE CASCADE,
    tenant        text        NOT NULL,
    sujeto        text        NOT NULL,
    concedido_por text        NOT NULL,
    creado        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (notebook_id, sujeto)
);

CREATE TABLE note_personas (
    note_id       uuid        NOT NULL REFERENCES notes (id) ON DELETE CASCADE,
    tenant        text        NOT NULL,
    sujeto        text        NOT NULL,
    puede_editar  boolean     NOT NULL DEFAULT false,
    concedido_por text        NOT NULL,
    creado        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (note_id, sujeto)
);

-- «Compartido conmigo» busca por persona, no por nota.
CREATE INDEX note_personas_sujeto ON note_personas (tenant, sujeto);

ALTER TABLE notebook_lectores ENABLE ROW LEVEL SECURITY;
ALTER TABLE notebook_lectores FORCE ROW LEVEL SECURITY;
CREATE POLICY notebook_lectores_tenant_isolation ON notebook_lectores
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY notebook_lectores_platform_merge ON notebook_lectores
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

ALTER TABLE note_personas ENABLE ROW LEVEL SECURITY;
ALTER TABLE note_personas FORCE ROW LEVEL SECURITY;
CREATE POLICY note_personas_tenant_isolation ON note_personas
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY note_personas_platform_merge ON note_personas
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
