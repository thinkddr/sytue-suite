-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Al revertir se pierden las listas y quién creó cada conexión. El binario anterior no
-- las mira: vuelve a dejar consultar a cualquier miembro de la organización (NEG-06).

DROP TABLE IF EXISTS instance_readers;
ALTER TABLE app_instances DROP COLUMN IF EXISTS created_by;
