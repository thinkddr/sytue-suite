-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Valla de escritura de la fuente privada del ERP. La 0103 congela la fuente
-- (erp_freeze_private_storage copia a la instantánea las filas de `records` de
-- una persona) pero nada impedía después escribir en esas mismas filas: un
-- INSERT, UPDATE o DELETE posterior al congelado cambiaba el ERP legacy sin que
-- la instantánea —ni la réplica cifrada que los dispositivos verifican contra
-- ella— se enterase. Con esta migración la base rechaza (PST01,
-- private_storage_legacy_write_blocked) cualquier escritura que toque filas de
-- una persona cuya fuente no está en `legacy` (building, frozen o private),
-- igual que contacts_private_row_guard en contacts.
--
-- Solo resuelve el dueño. El bloqueo de fila, la detección de conflictos en
-- REPEATABLE READ y la regla de «no se escribe tras congelar» son de la función
-- compartida sytue_private_storage_guard (platform/pkg/pg).
--
-- El dueño y el ámbito se resuelven EXACTAMENTE como en
-- erp_freeze_private_storage: fila de `records` sin deleted_at y con
-- coalesce(data->>'owner_sub', data->>'responsable_sub', '') no vacío. Se mira
-- la fila vieja (lo que pudo estar en la instantánea: editarla, borrarla o
-- reasignarla) y la nueva (lo que entraría en ella: crearla, restaurarla o
-- asignársela a una fuente congelada). Una fila borrada o sin dueño no forma
-- parte de ninguna fuente y sigue libre.
--
-- erp_freeze_private_storage y sytue_private_storage_release no escriben en
-- `records`, así que la valla no les afecta. Las tareas en lote que recorran
-- muchos dueños deben saltarse los congelados con sytue_private_storage_frozen
-- para que uno solo no aborte el lote entero.
CREATE FUNCTION erp_private_row_guard() RETURNS trigger LANGUAGE plpgsql
 SET search_path = pg_catalog, public AS $$
DECLARE
 fila jsonb;
 dueno text;
 duenos text[] := ARRAY[]::text[];
 tenant_fila text;
BEGIN
 FOR fila IN SELECT v FROM (VALUES
   (CASE WHEN TG_OP <> 'INSERT' THEN to_jsonb(OLD) END),
   (CASE WHEN TG_OP <> 'DELETE' THEN to_jsonb(NEW) END)) x(v) WHERE v IS NOT NULL
 LOOP
  -- Mismo predicado que la copia de erp_freeze_private_storage.
  CONTINUE WHEN fila->>'deleted_at' IS NOT NULL;
  dueno := coalesce(fila->'data'->>'owner_sub', fila->'data'->>'responsable_sub', '');
  CONTINUE WHEN dueno = '';
  -- El tenant va con el dueño: un UPDATE que cambie de tenant (fusión) se
  -- comprueba en los dos lados.
  duenos := duenos || ARRAY[fila->>'tenant', dueno];
 END LOOP;
 -- Orden fijo (tenant, dueño) y sin repetidos: dos escrituras que toquen los
 -- mismos dueños toman los bloqueos en el mismo orden.
 FOR tenant_fila, dueno IN
   SELECT DISTINCT duenos[i], duenos[i + 1]
     FROM generate_series(1, coalesce(array_length(duenos, 1), 0), 2) AS i
    ORDER BY 1, 2
 LOOP
  PERFORM public.sytue_private_storage_guard('public.erp_private_storage_states'::regclass, tenant_fila, dueno);
 END LOOP;
 IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$;

CREATE TRIGGER erp_private_records_guard BEFORE INSERT OR UPDATE OR DELETE ON records
 FOR EACH ROW EXECUTE FUNCTION erp_private_row_guard();
