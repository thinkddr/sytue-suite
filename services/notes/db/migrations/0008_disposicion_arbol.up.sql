-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- LA VISTA POR DEFECTO ES EL ÁRBOL: notas a la izquierda, la nota a la derecha,
-- como Mail con sus hilos y como el BasKet clásico. El lienzo libre, las columnas
-- y la lista siguen existiendo como disposiciones opcionales.
--
-- La restricción venía en línea en CREATE TABLE y Postgres la nombró él; se
-- localiza por su definición en vez de fiarse del nombre, porque un DROP a un
-- nombre equivocado con IF EXISTS no falla: deja la restricción vieja puesta y
-- la primera escritura con 'tree' revienta en producción.
DO $$
DECLARE c text;
BEGIN
    SELECT conname INTO c
      FROM pg_constraint
     WHERE conrelid = 'notebooks'::regclass AND contype = 'c'
       AND pg_get_constraintdef(oid) LIKE '%layout%';
    IF c IS NOT NULL THEN
        EXECUTE format('ALTER TABLE notebooks DROP CONSTRAINT %I', c);
    END IF;
END $$;

ALTER TABLE notebooks
    ADD CONSTRAINT notebooks_layout_check
    CHECK (layout IN ('tree', 'free', 'columns', 'list'));

ALTER TABLE notebooks ALTER COLUMN layout SET DEFAULT 'tree';
