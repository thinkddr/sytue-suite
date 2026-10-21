-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte la política de mantenimiento de organizations (deja intacta la de
-- aislamiento por tenant de 0001).
DROP POLICY IF EXISTS organizations_maintenance ON organizations;
