-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira la política del enlace público. El aislamiento por organización no
-- depende de ella; lo que se pierde es la lectura sin sesión de los cuadernos
-- publicados, que vuelve a responder «no existe».
-- sytue:destructive-ok solo elimina una política PERMISSIVE que amplía lecturas.

DROP POLICY IF EXISTS notebooks_enlace_publico ON notebooks;
