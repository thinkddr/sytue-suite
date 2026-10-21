-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- FIRMA del webhook SALIENTE + dialecto Teams saliente.
--
-- Hasta ahora el saliente solo llevaba `X-Sytue-Webhook-Token`, que viaja EN CLARO en
-- cada entrega: quien vea una petición puede forjar las siguientes. Un bot escrito con
-- Bolt o slack_sdk ni lo mira —valida `X-Slack-Signature`— y responde 401. Con un
-- secreto de firma aparte (que NUNCA viaja) el receptor puede comprobar de verdad quién
-- envía y que el cuerpo no se ha tocado.
--
-- El secreto se guarda en base64 a propósito: Slack lo usa como cadena tal cual y Teams
-- lo base64-decodifica para obtener la clave. UN solo valor sirve para los dos dialectos.
ALTER TABLE outgoing_webhooks
    ADD COLUMN signing_secret text NOT NULL DEFAULT '';

-- Los salientes ya creados nacen con un secreto propio, no vacío: si no, el admin no
-- tendría nada que pegar en su bot hasta recrear el webhook. gen_random_uuid() es del
-- núcleo de PostgreSQL (no exige pgcrypto); dos concatenados dan 244 bits de entropía.
UPDATE outgoing_webhooks
   SET signing_secret = encode(decode(
           replace(gen_random_uuid()::text, '-', '') ||
           replace(gen_random_uuid()::text, '-', ''), 'hex'), 'base64')
 WHERE signing_secret = '';

-- 'teams' como dialecto saliente: el POST toma la forma de la actividad que Microsoft
-- Teams envía a un outgoing webhook, para que un bot escrito para Teams funcione contra
-- Rooms sin tocarlo (el entrante ya lo aceptaba desde 0017).
ALTER TABLE outgoing_webhooks
    DROP CONSTRAINT outgoing_webhooks_dialect_known,
    ADD CONSTRAINT outgoing_webhooks_dialect_known CHECK (dialect IN ('sytue', 'slack', 'teams'));
