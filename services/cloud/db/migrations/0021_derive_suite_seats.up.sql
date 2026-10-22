-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Los puestos de la suite se DERIVAN de los miembros de la organización, no se
-- teclean. La 0020 los había guardado en organizations.suite_seats a la espera de
-- decidir el origen; decidido que salen de la membresía, esa columna sobra: sería
-- una segunda fuente de verdad que nadie escribe y que quedaría desfasada en
-- cuanto entrara o saliera alguien del equipo.
--
-- Ahora el conteo vive en `memberships` (una fila por persona con acceso a la
-- organización) y se lee en el momento de facturar y de comprobar el requisito.
--
-- sytue:destructive-ok la columna la introdujo la 0020 y ningún binario llegó a
-- escribirla ni a leerla (nada en services/cloud la nombra), así que retirarla no
-- rompe a la versión anterior.

ALTER TABLE organizations DROP CONSTRAINT IF EXISTS organizations_suite_seats_nonneg;
ALTER TABLE organizations DROP COLUMN IF EXISTS suite_seats;
