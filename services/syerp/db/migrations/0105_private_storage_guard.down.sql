-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Quitar la valla con una fuente congelada reabriría la escritura sobre filas
-- que ya están en una instantánea: se exige que todas vuelvan a `legacy` antes.
-- La tabla de estados tiene FORCE ROW LEVEL SECURITY: sin app.maintenance un
-- dueño sin superusuario no vería ninguna fila y la comprobación pasaría.
DO $$
DECLARE previo text := current_setting('app.maintenance', true);
BEGIN
 PERFORM set_config('app.maintenance', 'on', true);
 IF EXISTS (SELECT 1 FROM erp_private_storage_states WHERE mode <> 'legacy') THEN
  RAISE EXCEPTION 'la valla de la fuente privada del ERP no se retira con una fuente congelada';
 END IF;
 PERFORM set_config('app.maintenance', coalesce(previo, ''), true);
END $$;
DROP TRIGGER erp_private_records_guard ON records;
DROP FUNCTION erp_private_row_guard();
