-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Los cuadernos en árbol vuelven a columnas antes de estrechar la restricción:
-- si no, el ADD CONSTRAINT falla con la primera fila que la incumpla. La
-- actualización cruza organizaciones, así que va bajo la política de fusión
-- (`app.merge`), que es la puerta prevista para escrituras de plataforma.
SET app.merge = 'on';
UPDATE notebooks SET layout = 'columns' WHERE layout = 'tree';
RESET app.merge;

ALTER TABLE notebooks DROP CONSTRAINT IF EXISTS notebooks_layout_check;
ALTER TABLE notebooks
    ADD CONSTRAINT notebooks_layout_check
    CHECK (layout IN ('free', 'columns', 'list'));
ALTER TABLE notebooks ALTER COLUMN layout SET DEFAULT 'columns';
