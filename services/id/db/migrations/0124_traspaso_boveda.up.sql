-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El traspaso de la autoridad del correo de Matrix a la bóveda (ADR 0087).
--
-- `reason` gana `traspaso`: la bóveda pasa a ser la autoridad del correo de quien
-- ya no puede abrir su correo de Matrix. Se registra siempre con identificación
-- reciente, también en un alta; por eso puede ser la generación 1. Rooms da la
-- autoridad de la bóveda sobre una de Matrix solo si la clave activa tiene este
-- motivo y es posterior al último cambio de la autoridad de Matrix.
--
-- Antes: la generación 1 era siempre `alta`, y `alta` siempre la 1. Ahora la 1 es
-- `alta` o `traspaso`, y `alta` sigue siendo solo la 1. Solo el correo admite el
-- traspaso: es el único producto que recibe por la bóveda.
--
-- sytue:destructive-ok Las dos restricciones que se quitan se sustituyen en la misma
-- migración por otras que solo ADMITEN más (el motivo `traspaso` en la generación
-- 1); todo lo que escribe la versión anterior del servicio sigue siendo válido.
ALTER TABLE e2e_vault_keys
    DROP CONSTRAINT e2e_vault_keys_reason_check,
    DROP CONSTRAINT e2e_vault_keys_check;

ALTER TABLE e2e_vault_keys
    ADD CONSTRAINT e2e_vault_keys_motivo
        CHECK (reason IN ('alta','continuidad','vaciado','traspaso')),
    ADD CONSTRAINT e2e_vault_keys_alta_es_la_primera
        CHECK (reason <> 'alta' OR generation = 1),
    ADD CONSTRAINT e2e_vault_keys_primera_es_alta_o_traspaso
        CHECK (generation <> 1 OR reason IN ('alta','traspaso')),
    ADD CONSTRAINT e2e_vault_keys_traspaso_solo_correo
        CHECK (reason <> 'traspaso' OR product = 'mail');
