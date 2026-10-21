-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Conexiones a terceros de SyERP: Stripe y Salesforce.
--
-- REPARTO, y es el motivo de que esta tabla sea tan pequeña: TODO lo que SALE
-- hacia un tercero sale por el Sytue Integration Gateway (:8093), que ya tiene
-- el único cliente de egress con SSRF, límite, auditoría y secretos sellados.
-- Aquí solo vive lo que la pasarela no cubre porque no es su trabajo: qué
-- instancia suya usa esta organización, y el secreto de firma de los webhooks
-- ENTRANTES —la pasarela es de salida—.
--
-- El secreto va sellado con platform/pkg/keycrypto (AES-256-GCM). En claro no
-- se guarda ni se registra: con él se pueden falsificar cobros.

CREATE TABLE conexiones (
    tenant      text        NOT NULL,
    proveedor   text        NOT NULL CHECK (proveedor IN ('stripe', 'salesforce')),

    -- Identificador de la instancia en el Integration Gateway, para el egress.
    -- Vacío en un Stripe que solo recibe webhooks.
    instancia   text        NOT NULL DEFAULT '',

    -- Secreto de firma del webhook, sellado. Vacío = este proveedor no entra.
    secreto_enc text        NOT NULL DEFAULT '',

    -- Config PÚBLICA: diario y cuenta de cobro, moneda. Sin secretos.
    config      jsonb       NOT NULL DEFAULT '{}'::jsonb,

    activo      boolean     NOT NULL DEFAULT true,
    creado_por  text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (tenant, proveedor),
    CONSTRAINT conexiones_config_es_objeto CHECK (jsonb_typeof(config) = 'object')
);

-- ---------------------------------------------------------------------------
-- eventos_entrantes — los webhooks ya procesados, por id de evento del
-- proveedor.
--
-- Stripe reintenta: el MISMO evento llega varias veces por diseño. Sin esta
-- tabla, «factura pagada» se cobraría dos veces y el libro llevaría dos asientos
-- de cobro. La clave primaria es la que garantiza el «una sola vez», no un `if`
-- en el código: dos reintentos simultáneos chocan aquí.
-- ---------------------------------------------------------------------------
CREATE TABLE eventos_entrantes (
    tenant     text        NOT NULL,
    proveedor  text        NOT NULL,
    evento_id  text        NOT NULL,
    tipo       text        NOT NULL DEFAULT '',
    recibido   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, proveedor, evento_id)
);

-- Purga: los eventos viejos ya no pueden reintentarse (Stripe deja de intentar
-- a los 3 días). El índice permite barrerlos sin escanear la tabla.
CREATE INDEX eventos_entrantes_purga ON eventos_entrantes (recibido);

ALTER TABLE conexiones        ENABLE ROW LEVEL SECURITY;
ALTER TABLE eventos_entrantes ENABLE ROW LEVEL SECURITY;
ALTER TABLE conexiones        FORCE ROW LEVEL SECURITY;
ALTER TABLE eventos_entrantes FORCE ROW LEVEL SECURITY;

CREATE POLICY conexiones_tenant_isolation ON conexiones
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY eventos_entrantes_tenant_isolation ON eventos_entrantes
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
