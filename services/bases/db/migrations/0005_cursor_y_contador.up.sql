-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Paginación por cursor y fin del techo de 50.000 registros por tabla (ADR 0068, bases-2).
--
-- El techo existía porque dos consultas costaban lo que medía la tabla: el OFFSET de
-- una página profunda y el `count(*)` que cada alta hacía para comprobarlo. La página
-- va ahora por cursor (el índice `base_records_por_tabla` ya la sostiene en el orden
-- por defecto) y el número de registros se lleva en `base_tables.record_count`, que
-- mantiene un trigger: así lo cumplen TODAS las vías de alta y baja (rejilla,
-- formulario público, importación, cascadas), sin que ninguna tenga que acordarse.
ALTER TABLE base_tables ADD COLUMN record_count bigint NOT NULL DEFAULT 0 CHECK (record_count >= 0);

CREATE FUNCTION base_records_contar() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    -- En una cascada desde la tabla, su fila ya no existe y el UPDATE no toca nada.
    IF TG_OP = 'INSERT' THEN
        UPDATE base_tables t SET record_count = t.record_count + n.c
          FROM (SELECT tenant, table_id, count(*) AS c FROM cambiadas GROUP BY tenant, table_id) n
         WHERE t.tenant = n.tenant AND t.id = n.table_id;
    ELSE
        UPDATE base_tables t SET record_count = greatest(t.record_count - n.c, 0)
          FROM (SELECT tenant, table_id, count(*) AS c FROM cambiadas GROUP BY tenant, table_id) n
         WHERE t.tenant = n.tenant AND t.id = n.table_id;
    END IF;
    RETURN NULL;
END $$;

CREATE TRIGGER base_records_contar_altas AFTER INSERT ON base_records
    REFERENCING NEW TABLE AS cambiadas FOR EACH STATEMENT EXECUTE FUNCTION base_records_contar();
CREATE TRIGGER base_records_contar_bajas AFTER DELETE ON base_records
    REFERENCING OLD TABLE AS cambiadas FOR EACH STATEMENT EXECUTE FUNCTION base_records_contar();

-- Lo que ya existía. La RLS va forzada también para quien migra: se cuenta con la
-- política de fusión, que es la única que ve todas las organizaciones.
SELECT set_config('app.merge', 'on', true);
UPDATE base_tables t SET record_count = n.c
  FROM (SELECT tenant, table_id, count(*) AS c FROM base_records GROUP BY tenant, table_id) n
 WHERE t.tenant = n.tenant AND t.id = n.table_id;
SELECT set_config('app.merge', '', true);
