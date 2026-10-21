-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Bajar esto deja las operaciones por carácter de Docus sin validar contra las secciones
-- (el techo que tenía la 0016). Solo se pierde una copia: el documento no se toca.
ALTER TABLE doc_proteccion DROP COLUMN IF EXISTS secuencia;
