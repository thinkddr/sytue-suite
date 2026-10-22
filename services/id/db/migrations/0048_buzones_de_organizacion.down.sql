-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte los buzones de organización. Al caer la tabla se pierden las direcciones
-- asignadas y los alias; las cuentas ya creadas en Sytue Mail sobreviven con la
-- dirección que tuvieran, porque viven en su propia base.
DROP TABLE IF EXISTS org_mailboxes;

ALTER TABLE org_invitations DROP COLUMN IF EXISTS mailbox_local;
