-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- PLANTILLAS DE RESPUESTA, por cuenta y en el servidor.
--
-- Quien contesta cada día lo mismo (soporte, ventas, una secretaría) lo tenía que
-- reescribir o pegarlo de un documento aparte. Una plantilla es un texto con nombre
-- que el redactor inserta donde está el cursor, como las «Plantillas» de Gmail.
--
-- TABLA y no columna (a diferencia de la firma, 0014): son N por cuenta, cada una
-- con su nombre, y se crean, renombran y borran sueltas.
--
-- Texto plano, como la firma: el redactor lo escapa al convertirlo en HTML al
-- insertarlo. Guardar HTML aquí sería guardar entrada sin sanear para inyectarla
-- luego en el cuerpo de un correo.
--
-- Multi-tenant por RLS (app.tenant) como mail_labels, y owner-scoped por
-- account_id en cada consulta: una plantilla es de su cuenta, no de la
-- organización. La política de fusión de organizaciones (0010) se genera por
-- tabla al migrar; para esta tabla nueva se declara aquí abajo igual.

CREATE TABLE mail_templates (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    account_id  uuid        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    name        text        NOT NULL,
    body        text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    -- Los topes los impone el servicio con un 400 que el cliente sabe explicar;
    -- estos CHECK son la última línea frente a un UPDATE a mano.
    CONSTRAINT mail_templates_name_not_blank CHECK (length(btrim(name)) > 0),
    CONSTRAINT mail_templates_name_len CHECK (char_length(name) <= 100),
    CONSTRAINT mail_templates_body_len CHECK (octet_length(body) <= 16384)
);

-- Nombre único (sin distinguir mayúsculas) por cuenta: en el menú del redactor dos
-- «Horario» iguales no se distinguen.
CREATE UNIQUE INDEX mail_templates_unique_name ON mail_templates (account_id, lower(name));
CREATE INDEX mail_templates_account_idx ON mail_templates (tenant, account_id);

ALTER TABLE mail_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_templates FORCE  ROW LEVEL SECURITY;

CREATE POLICY mail_templates_tenant_isolation ON mail_templates
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- La de fusión de organizaciones (ver 0010): sin ella, fusionar dos organizaciones
-- dejaría las plantillas atrás, invisibles para el motor de fusión.
CREATE POLICY mail_templates_platform_merge ON mail_templates
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
