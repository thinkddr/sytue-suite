-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- «BORRAR MI CUENTA» (ADR 0054): la baja que pide la propia persona.
--
-- Se apoya en la baja de la 0092 —`closed_at`, `closed_by`, `close_reason`— y
-- en su ventana de 30 días. Una baja propia se reconoce porque `closed_by` es la
-- misma cuenta: Sytue Admin no deja a nadie darse de baja a sí mismo
-- (adminMayManage) y SCIM no escribe `closed_by`, así que no hay otra forma de
-- llegar a ese estado.
--
-- Las dos columnas son los dos pasos que quedan después de confirmar, y cada una
-- es la cola de su barrido (BarrerCuentasBorradas):
--
--   * `personal_data_erased_at`: cuándo se borró lo que otros servicios guardan
--     de ESTA persona y no de su organización (avisos del Hub, suscripciones de
--     push de Rooms, favoritos de Drive). Se hace en cuanto se puede, no al
--     vencer la ventana: una suscripción de push de alguien que se ha ido no
--     tiene nada que devolver si se restaura la cuenta.
--   * `purged_at`: cuándo, vencida la ventana, se anonimizó la ficha de Sytue ID
--     (correo, nombre, móvil, @usuario...) y se quitaron sus membresías.
--
-- Sin tabla nueva con `tenant`, así que no hace falta política de fusión.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS personal_data_erased_at timestamptz,
    ADD COLUMN IF NOT EXISTS purged_at               timestamptz;

-- La cola de los dos barridos: solo bajas propias sin terminar, que son una
-- fracción diminuta de la tabla.
CREATE INDEX IF NOT EXISTS users_baja_propia_pendiente_idx ON users (closed_at)
    WHERE status = 'deleted' AND closed_by = id AND purged_at IS NULL;
