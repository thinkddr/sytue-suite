-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- COMPATIBILIDAD Slack/Teams: dialecto del webhook SALIENTE. 'sytue' (por defecto)
-- mantiene el POST JSON histórico; 'slack' emite el form-urlencoded de un outgoing
-- webhook de Slack y lee su respuesta {text, response_type}, para que un bot escrito
-- para Slack funcione contra Rooms sin tocarlo.
--
-- El webhook ENTRANTE no necesita columna: autodetecta el dialecto del cuerpo
-- (nativo / Slack / Teams) y los alias /hooks/slack|teams/{token} lo fuerzan.
ALTER TABLE outgoing_webhooks
    ADD COLUMN dialect text NOT NULL DEFAULT 'sytue',
    ADD CONSTRAINT outgoing_webhooks_dialect_known CHECK (dialect IN ('sytue', 'slack'));
