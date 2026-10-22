-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Permisos (bases-2, colaboración B): un GRUPO de Sytue ID como principal de la ACL, por
-- su nombre (el que el token trae en `groups`), y la etiqueta con la que se enseña una
-- persona en la lista de permisos. Solo ensancha: el binario anterior ignora una entrada
-- de una clase que no conoce (deny-by-default) y no lee la columna nueva.
ALTER TABLE base_permissions
    DROP CONSTRAINT base_permissions_principal_kind_check,
    ADD CONSTRAINT base_permissions_principal_kind_check CHECK (principal_kind IN ('user', 'group', 'tenant')),
    ADD COLUMN principal_label text NOT NULL DEFAULT '' CHECK (length(principal_label) <= 200);
