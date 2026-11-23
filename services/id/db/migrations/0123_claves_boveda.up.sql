-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El registro de las claves de firma de las bóvedas (ADR 0087).
--
-- Cada bóveda post-cuántica de una persona, por organización y producto, guarda
-- dentro de su secreto una clave de firma híbrida (Ed25519 + ML-DSA-65). Aquí solo
-- se guarda su parte PÚBLICA: es el ancla con la que un servicio (el correo
-- entrante) sabe que una acción viene de la persona. Una fila por generación; la
-- activa es la de mayor generación y `retired_at` nulo. Nada se borra: el historial
-- es lo que la persona puede revisar para saber si alguien puso una clave suya.
--
-- `reason`:
--   alta        — la primera clave de esa bóveda.
--   continuidad — la nueva la aprobó la anterior con su firma.
--   vaciado     — la nueva sustituye a una que ya no se puede abrir; solo con
--                 identificación reciente, y se avisa a la persona por correo.
CREATE TABLE e2e_vault_keys (
    user_id         uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant          text        NOT NULL CHECK (tenant <> ''),
    product         text        NOT NULL CHECK (product <> ''),
    generation      bigint      NOT NULL CHECK (generation >= 1),
    key_id          text        NOT NULL CHECK (key_id ~ '^[0-9a-f]{64}$'),
    previous_key_id text        NOT NULL DEFAULT '' CHECK (previous_key_id = '' OR previous_key_id ~ '^[0-9a-f]{64}$'),
    public_key      jsonb       NOT NULL,
    reason          text        NOT NULL CHECK (reason IN ('alta','continuidad','vaciado')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    retired_at      timestamptz,
    PRIMARY KEY (user_id, tenant, product, generation),
    CHECK ((generation = 1) = (reason = 'alta')),
    CHECK ((generation = 1) = (previous_key_id = ''))
);

-- Una sola activa por bóveda.
CREATE UNIQUE INDEX e2e_vault_keys_activa
    ON e2e_vault_keys (user_id, tenant, product) WHERE retired_at IS NULL;
