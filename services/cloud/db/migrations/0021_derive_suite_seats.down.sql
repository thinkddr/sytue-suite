-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Restaura la columna de la 0020 (valor manual). Los datos previos no se
-- recuperan: se repuebla con el número de miembros, que es el criterio vigente.

ALTER TABLE organizations ADD COLUMN suite_seats integer NOT NULL DEFAULT 0;
UPDATE organizations o
   SET suite_seats = (SELECT count(*) FROM memberships m WHERE m.tenant = o.tenant);
ALTER TABLE organizations
    ADD CONSTRAINT organizations_suite_seats_nonneg CHECK (suite_seats >= 0);
