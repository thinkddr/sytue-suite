-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Vuelve a las reglas de 0123. Si ya hay algún traspaso registrado, falla: el
-- historial de claves no se borra para poder bajar.
ALTER TABLE e2e_vault_keys
    DROP CONSTRAINT e2e_vault_keys_motivo,
    DROP CONSTRAINT e2e_vault_keys_alta_es_la_primera,
    DROP CONSTRAINT e2e_vault_keys_primera_es_alta_o_traspaso,
    DROP CONSTRAINT e2e_vault_keys_traspaso_solo_correo;

ALTER TABLE e2e_vault_keys
    ADD CONSTRAINT e2e_vault_keys_reason_check
        CHECK (reason IN ('alta','continuidad','vaciado')),
    ADD CONSTRAINT e2e_vault_keys_check
        CHECK ((generation = 1) = (reason = 'alta'));
