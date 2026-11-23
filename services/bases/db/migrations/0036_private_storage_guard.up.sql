-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Valla de escritura de la fuente privada de Bases (como contacts en su 0009).
--
-- 0033 congela la fuente (bases_freeze_private_storage) pero nada impedía después
-- escribir en las filas que ya salieron hacia los dispositivos: un cambio en el
-- servidor se perdía en la copia E2E y, al «soltar la fuente» (0035), la vuelta a
-- legacy ya no sería «los datos tal como estaban al congelar».
--
-- La regla es la compartida de platform/pkg/pg (sytue_private_storage_guard): un
-- FOR SHARE sobre la fila de estado del propietario, que serializa con el FOR UPDATE
-- de la congelación, y PST01 si no es legacy. Aquí solo se resuelve QUIÉN es el
-- propietario: el de la base (`bases.owner_sub`), también cuando escribe otro miembro.
-- Cada hija lleva `base_id` salvo base_links, que lo saca de su tabla de origen.
--
-- Bases compartidas: la congelación SOLO captura las bases sin ninguna entrada de
-- permisos ajena (own_bases en 0033). Una base compartida no sale con su propietario
-- y sigue viviendo en el servidor; si se vallase, quedaría de solo lectura para todos
-- sus miembros sin tener copia E2E en ningún sitio. Por eso, con el propietario ya
-- fuera de legacy, se rechaza:
--   · lo que toca una base capturada en la instantánea de su época vigente, y
--   · el alta de una base nueva a su nombre (sus datos nuevos van a los dispositivos);
-- y se deja pasar lo que toca una base compartida que no se capturó. Como
-- base_permissions también está vallada, una base capturada no puede compartirse
-- después: el conjunto capturado no cambia mientras dure la congelación.
--
-- Sin bypass por app.maintenance (como en la función compartida). La congelación y
-- la liberación no escriben en estas tablas; una fusión (app.merge) pasa cuando el
-- propietario no tiene estado y se rechaza si está congelado, igual que en contacts.
-- Las tablas que no salen en la instantánea (comentarios, adjuntos, scripts,
-- formularios, conectores, auditoría, presencia, recálculo) no se vallan aquí.

CREATE FUNCTION bases_private_owner_guard(p_tenant text, p_owner_sub text, p_base uuid, p_nueva boolean)
RETURNS void LANGUAGE plpgsql
SET search_path = pg_catalog, public SET app.maintenance = 'on' AS $$
DECLARE v_mode text; v_epoch uuid;
BEGIN
 SELECT s.mode, s.epoch INTO v_mode, v_epoch
  FROM public.bases_private_storage_states s
  WHERE s.tenant = p_tenant AND s.owner_sub = p_owner_sub
  FOR SHARE;
 -- Base compartida que no salió con su propietario: sigue siendo del servidor. El
 -- FOR SHARE de arriba ya serializa con una congelación en curso.
 IF v_mode IN ('frozen', 'private') AND NOT p_nueva AND NOT EXISTS (
   SELECT 1 FROM public.bases_private_storage_snapshot sn
    WHERE sn.tenant = p_tenant AND sn.owner_sub = p_owner_sub AND sn.epoch = v_epoch
      AND sn.kind = 'base' AND sn.id = p_base::text) THEN
  RETURN;
 END IF;
 PERFORM public.sytue_private_storage_guard('public.bases_private_storage_states'::regclass, p_tenant, p_owner_sub);
END;
$$;

CREATE FUNCTION bases_private_row_guard() RETURNS trigger LANGUAGE plpgsql
SET search_path = pg_catalog, public SET app.maintenance = 'on' AS $$
DECLARE value jsonb; v_base uuid; candidate record;
BEGIN
 FOR value IN SELECT v FROM (VALUES
   (CASE WHEN TG_OP <> 'INSERT' THEN to_jsonb(OLD) END),
   (CASE WHEN TG_OP <> 'DELETE' THEN to_jsonb(NEW) END)) x(v) WHERE v IS NOT NULL
 LOOP
  IF TG_TABLE_NAME = 'bases' THEN
   -- La propia fila dice el propietario: el anterior y el nuevo en una transferencia.
   PERFORM public.bases_private_owner_guard(value->>'tenant', value->>'owner_sub',
     (value->>'id')::uuid, TG_OP = 'INSERT');
  ELSE
   IF TG_TABLE_NAME = 'base_links' THEN
    SELECT t.base_id INTO v_base FROM public.base_tables t
     WHERE t.tenant = value->>'tenant' AND t.id = (value->>'from_table_id')::uuid;
   ELSE
    v_base := (value->>'base_id')::uuid;
   END IF;
   -- En una cascada desde la base el padre ya no se ve: su propio disparador ya valló.
   FOR candidate IN SELECT b.tenant, b.owner_sub, b.id FROM public.bases b
     WHERE b.tenant = value->>'tenant' AND b.id = v_base
   LOOP
    PERFORM public.bases_private_owner_guard(candidate.tenant, candidate.owner_sub, candidate.id, false);
   END LOOP;
  END IF;
 END LOOP;
 IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$;

CREATE TRIGGER bases_private_bases_guard BEFORE INSERT OR UPDATE OR DELETE ON bases
 FOR EACH ROW EXECUTE FUNCTION bases_private_row_guard();
CREATE TRIGGER bases_private_permissions_guard BEFORE INSERT OR UPDATE OR DELETE ON base_permissions
 FOR EACH ROW EXECUTE FUNCTION bases_private_row_guard();
CREATE TRIGGER bases_private_tables_guard BEFORE INSERT OR UPDATE OR DELETE ON base_tables
 FOR EACH ROW EXECUTE FUNCTION bases_private_row_guard();
CREATE TRIGGER bases_private_fields_guard BEFORE INSERT OR UPDATE OR DELETE ON base_fields
 FOR EACH ROW EXECUTE FUNCTION bases_private_row_guard();
CREATE TRIGGER bases_private_views_guard BEFORE INSERT OR UPDATE OR DELETE ON base_views
 FOR EACH ROW EXECUTE FUNCTION bases_private_row_guard();
CREATE TRIGGER bases_private_records_guard BEFORE INSERT OR UPDATE OR DELETE ON base_records
 FOR EACH ROW EXECUTE FUNCTION bases_private_row_guard();
CREATE TRIGGER bases_private_links_guard BEFORE INSERT OR UPDATE OR DELETE ON base_links
 FOR EACH ROW EXECUTE FUNCTION bases_private_row_guard();
