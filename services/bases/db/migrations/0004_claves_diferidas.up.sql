-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Todas las claves foráneas de Bases pasan a comprobarse AL CONFIRMAR.
--
-- La fusión de organizaciones de Sytue ID reescribe `tenant` tabla a tabla, en el orden
-- del catálogo (`UPDATE <tabla> SET tenant = …`). Con claves compuestas (tenant, id)
-- inmediatas, la primera tabla hija que se actualiza antes que su padre viola su clave y
-- la fusión aborta. Diferidas, cada UPDATE pasa y al confirmar todo vuelve a cuadrar (el
-- ON UPDATE CASCADE del padre ya no encuentra nada que arrastrar, y no estorba).
--
-- No cambia lo que las claves garantizan: un enlace entre organizaciones sigue sin poder
-- confirmarse, y el borrado seguro ya iba diferido desde 0001.
DO $$
DECLARE
    c record;
BEGIN
    FOR c IN
        SELECT conrelid::regclass AS tabla, conname
          FROM pg_constraint
         WHERE contype = 'f'
           AND connamespace = current_schema()::regnamespace
           AND conrelid::regclass::text IN (
               'base_permissions', 'base_tables', 'base_fields', 'base_records',
               'base_links', 'base_views', 'base_form_links', 'base_connections')
    LOOP
        EXECUTE format('ALTER TABLE %s ALTER CONSTRAINT %I DEFERRABLE INITIALLY DEFERRED', c.tabla, c.conname);
    END LOOP;
END $$;
