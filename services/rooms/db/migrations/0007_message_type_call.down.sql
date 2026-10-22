-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Reversión. Nota: re-imponer el CHECK estricto (length(body) > 0) fallará si ya
-- se insertaron mensajes «call» o solo-adjuntos con cuerpo vacío tras la subida;
-- purga o rellena esas filas antes de revertir. Es el riesgo inherente de estrechar
-- un invariante en un rollback.

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_body_not_blank;
ALTER TABLE messages ADD CONSTRAINT messages_body_not_blank CHECK (length(body) > 0);

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_type_valid;
ALTER TABLE messages DROP COLUMN IF EXISTS type;
