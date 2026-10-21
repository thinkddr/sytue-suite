-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Wave 8 — integración GitHub App (repos privados + comentario de preview en PR).
--
-- 1) projects.github_installation_id: id de la instalación de la GitHub App para
--    el repo del proyecto (0 = sin App conectada; el proyecto usa deploy-key SSH
--    o repo público). NO es sensible: es un entero inútil sin la clave privada de
--    la App (que vive solo en el entorno/config, nunca en la BD). Por eso se
--    guarda en claro (nada que cifrar aquí).
-- 2) github_pr_comments: dedupe/idempotencia del comentario de preview por
--    (project_id, pr_number). El notificador RECUERDA el id del comentario que
--    publicó, de modo que un re-deploy del MISMO PR EDITA ese comentario en vez
--    de crear uno nuevo. FK ON DELETE CASCADE ata su vida a la del proyecto.
--    RLS deny-by-default por tenant (patrón 0001); el worker fija app.tenant al
--    tenant del evento antes de leer/escribir.

ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS github_installation_id bigint NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS github_pr_comments (
    tenant     text        NOT NULL,
    project_id uuid        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    pr_number  integer     NOT NULL,
    comment_id bigint      NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, pr_number)
);

ALTER TABLE github_pr_comments ENABLE ROW LEVEL SECURITY;
ALTER TABLE github_pr_comments FORCE ROW LEVEL SECURITY;

CREATE POLICY github_pr_comments_tenant_isolation ON github_pr_comments
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
