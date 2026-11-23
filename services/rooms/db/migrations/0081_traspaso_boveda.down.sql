-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Un traspaso hecho no se deshace bajando: devolvería la autoridad a una raíz de
-- Matrix que la persona ya no puede abrir y aceptaría otra vez sus pruebas.
BEGIN;
SET LOCAL row_security=off;
LOCK TABLE matrix_storage_vault_handovers IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM matrix_storage_vault_handovers) THEN
  RAISE EXCEPTION 'vault handovers must be retained; rollback refused' USING ERRCODE='55000';
 END IF;
END $$;
DROP TABLE matrix_storage_vault_handovers;
DROP FUNCTION matrix_storage_vault_handover_guard();
COMMIT;
