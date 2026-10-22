-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Compartir una base con una persona de OTRA organización (bases-2, colaboración B;
-- precedente: ADR 0040 y 0052). La base, sus filas y su RLS se quedan en la organización
-- ANFITRIONA: la persona de fuera es una entrada más de la ACL, de clase `external`, con
-- su `sub`. Solo ensancha: el binario anterior ignora una clase que no conoce.
ALTER TABLE base_permissions
    DROP CONSTRAINT base_permissions_principal_kind_check,
    ADD CONSTRAINT base_permissions_principal_kind_check
        CHECK (principal_kind IN ('user', 'group', 'tenant', 'external'));

CREATE INDEX base_permissions_externos ON base_permissions (principal_id) WHERE principal_kind = 'external';

-- La ÚNICA lectura entre organizaciones: las entradas `external` de UNA persona, la que
-- la transacción declara en `app.bases_externo`. Solo SELECT, solo esa clase y solo sus
-- filas; sirve para saber en qué organizaciones es invitada. Ninguna otra tabla de Bases
-- se abre: lo demás se lee ya dentro de la anfitriona, con su ACL.
CREATE POLICY base_permissions_externo ON base_permissions FOR SELECT
    USING (principal_kind = 'external' AND principal_id = current_setting('app.bases_externo', true));
