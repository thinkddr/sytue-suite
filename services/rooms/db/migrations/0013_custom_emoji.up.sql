-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- EMOJI PERSONALIZADOS por tenant (Ola 14 · Extensibilidad de usuario).
--
-- Cada emoji liga un :nombre: (shortcode) a una imagen que YA vive en Sytue Drive
-- (file_id); no hay almacén de imágenes propio (mismo modelo que los adjuntos). La
-- imagen se sirve al navegador por el proxy de adjuntos (/api/attachment?fileId=)
-- con nosniff, así que un SVG con <script> nunca ejecuta. content_type se denormaliza
-- para poder filtrar (image/*) sin volver a llamar a Drive.
--
-- Catálogo ACOTADO por tenant: la PK (tenant, name) es a la vez el índice que cubre
-- el listado por tenant (prefijo tenant) sin N+1 ni índice extra. RLS deny-by-default
-- por tenant (mismo patrón que reactions/message_acks/read_state). El CHECK del nombre
-- (minúsculas/dígitos/_/-, 1..64) es la red de seguridad a nivel de esquema; el
-- servicio lo valida antes con domain.ValidEmojiName.

CREATE TABLE custom_emoji (
    tenant       text        NOT NULL,
    name         text        NOT NULL,
    file_id      uuid        NOT NULL,
    content_type text        NOT NULL DEFAULT '',
    created_by   text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, name),
    CONSTRAINT custom_emoji_name_valid CHECK (name ~ '^[a-z0-9_-]{1,64}$')
);

ALTER TABLE custom_emoji ENABLE ROW LEVEL SECURITY;
ALTER TABLE custom_emoji FORCE ROW LEVEL SECURITY;
CREATE POLICY custom_emoji_tenant_isolation ON custom_emoji
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
