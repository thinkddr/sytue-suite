-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sin efecto a propósito: la 0033 dejó el mismo destino en los demás clients y
-- quitarlo aquí desharía aquello. Revertir esta migración solo debe devolver el
-- esquema, y el esquema no cambia.
SELECT 1;
