-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
-- Bóveda PQ (ADR 0086, «Drive nativo ... pasa Matrix a ella»): versión y marca de copia.
--
-- `version` sube con cada registro nuevo y con cada vaciado, bajo el cerrojo de la
-- fila de la bóveda. Vaciar exige la (vault_id, version) que el cliente vio: si
-- alguien publicó algo después (un recibo de importación, por ejemplo), el vaciado
-- se rechaza en vez de borrarlo.
--
-- `copy_*` es la marca «copia en curso» que pone el cliente nativo antes de pasar
-- el Drive de Matrix a la bóveda: quién (etiqueta del dispositivo), qué (solo
-- `matrix-copy`), de qué época del servidor y hasta cuándo vale si no se renueva.
-- Mientras valga, solo quien la tiene publica o vacía; caduca sola o se cancela.
ALTER TABLE pq_vaults
    ADD COLUMN version bigint NOT NULL DEFAULT 0 CHECK (version >= 0),
    ADD COLUMN copy_id text CHECK (copy_id ~ '^[A-Za-z0-9_-]{22,64}$'),
    ADD COLUMN copy_holder text CHECK (length(copy_holder) BETWEEN 1 AND 128),
    ADD COLUMN copy_kind text CHECK (copy_kind IN ('matrix-copy')),
    ADD COLUMN copy_epoch text CHECK (length(copy_epoch) BETWEEN 1 AND 255),
    ADD COLUMN copy_started_at timestamptz,
    ADD COLUMN copy_expires_at timestamptz,
    ADD CONSTRAINT pq_vaults_copy_whole CHECK (
        num_nulls(copy_id, copy_holder, copy_kind, copy_epoch, copy_started_at, copy_expires_at) IN (0, 6)
    );
