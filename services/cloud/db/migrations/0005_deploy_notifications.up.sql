-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Ola B lote 2 — notificaciones de deploy por webhook saliente genérico
-- (Slack/Discord/genérico) y dedupe de envío.
--
-- 1) notify_webhook_url por ORGANIZACIÓN y por PROYECTO: URL destino del aviso
--    de deploy (ready/error). El worker resuelve project-first, luego org
--    (COALESCE), así una org configura un canal por defecto y un proyecto lo
--    sobreescribe. Validada anti-SSRF (domain.ValidateWebhookURL) al fijarla.
-- 2) deploy_notifications: dedupe/idempotencia por (deployment_id, status). El
--    worker RECLAMA la fila (INSERT ON CONFLICT DO NOTHING) antes de enviar, así
--    no re-envía la misma transición. FK ON DELETE CASCADE ata su vida a la del
--    deployment: la purga de previews retirados (0002) limpia también sus avisos,
--    sin código de retención propio (crecimiento acotado por deployments vivos).
--    RLS deny-by-default por tenant (patrón 0001); el worker fija app.tenant al
--    tenant del evento antes de reclamar.

ALTER TABLE organizations ADD COLUMN notify_webhook_url text NOT NULL DEFAULT '';
ALTER TABLE projects      ADD COLUMN notify_webhook_url text NOT NULL DEFAULT '';

CREATE TABLE deploy_notifications (
    tenant        text        NOT NULL,
    deployment_id uuid        NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    status        text        NOT NULL CHECK (status IN ('ready', 'error')),
    url           text        NOT NULL DEFAULT '',
    sent_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (deployment_id, status)
);

ALTER TABLE deploy_notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE deploy_notifications FORCE ROW LEVEL SECURITY;

CREATE POLICY deploy_notifications_tenant_isolation ON deploy_notifications
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
