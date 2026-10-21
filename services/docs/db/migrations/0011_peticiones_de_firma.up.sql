-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- «Pedir firma» de CertiPDF (docs/decisions.md#0051): una persona pide a otra de su
-- organización que firme un PDF de Drive.
--
-- El PDF NO se copia aquí: vive en Drive, en la unidad de la organización, y la fila
-- guarda su identificador y su ruta para enlazarlo. Quien firma lo abre, lo firma con
-- su certificado y deja el firmado en Drive; entonces la fila apunta también a ese
-- fichero, que el servicio ha comprobado que lleva una firma que cubre el documento
-- entero. Nombre y correo de quien pide se guardan como venían en SU token, para
-- enseñarlos sin preguntar a Sytue ID; el permiso va siempre contra el `sub`.
CREATE TABLE pdf_peticiones_firma (
    id                 uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant             text        NOT NULL,
    solicitante        text        NOT NULL,
    solicitante_nombre text        NOT NULL DEFAULT '',
    destinatario       text        NOT NULL,
    -- El correo de quien firma, tal como lo resolvió Sytue ID al pedir: para
    -- enseñar a quién se pidió sin volver a preguntar.
    destinatario_correo text       NOT NULL DEFAULT '',
    fichero            text        NOT NULL,
    ruta               text        NOT NULL,
    mensaje            text        NOT NULL DEFAULT '',
    estado             text        NOT NULL DEFAULT 'pendiente',
    creada             timestamptz NOT NULL DEFAULT now(),
    resuelta           timestamptz,
    firmado            text,
    firmado_ruta       text,
    CONSTRAINT pdf_peticiones_firma_estado_chk
        CHECK (estado IN ('pendiente', 'firmada', 'rechazada', 'cancelada')),
    CONSTRAINT pdf_peticiones_firma_firmado_chk
        CHECK ((estado = 'firmada') = (firmado IS NOT NULL)),
    CONSTRAINT pdf_peticiones_firma_mensaje_chk CHECK (char_length(mensaje) <= 1000),
    CONSTRAINT pdf_peticiones_firma_ruta_chk CHECK (char_length(ruta) <= 4096)
);

-- Una sola pendiente por fichero y destinatario: pedirla dos veces no manda dos avisos.
CREATE UNIQUE INDEX pdf_peticiones_firma_pendiente_uq
    ON pdf_peticiones_firma (tenant, fichero, destinatario) WHERE estado = 'pendiente';
CREATE INDEX pdf_peticiones_firma_destinatario_idx ON pdf_peticiones_firma (tenant, destinatario, creada DESC);
-- También sirve al tope por hora de quien pide (store.MaxPeticionesFirmaPorHora).
CREATE INDEX pdf_peticiones_firma_solicitante_idx ON pdf_peticiones_firma (tenant, solicitante, creada DESC);

-- Aislamiento por organización (deny-by-default), el mismo patrón que 0001 y 0007.
ALTER TABLE pdf_peticiones_firma ENABLE ROW LEVEL SECURITY;
ALTER TABLE pdf_peticiones_firma FORCE ROW LEVEL SECURITY;

CREATE POLICY pdf_peticiones_firma_tenant_isolation ON pdf_peticiones_firma
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Fusión de organizaciones (0004 solo cubrió las tablas que ya existían).
CREATE POLICY pdf_peticiones_firma_platform_merge ON pdf_peticiones_firma
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

GRANT SELECT, INSERT, UPDATE, DELETE ON pdf_peticiones_firma TO docs_app;
