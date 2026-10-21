-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Ola 18 — tokens de API confinados a un proyecto (paridad Scaleway/DO: API keys
-- por-recurso). ADITIVO Y FAIL-SAFE: project_id NULL = tenant-wide (EXACTAMENTE
-- el comportamiento previo, sin regresión); project_id fijado = el token solo
-- puede operar bajo /api/v1/projects/{project_id}/* (enforcement en el middleware
-- de auth). Acota el radio de impacto si un token de CI se filtra.

-- FK a projects(id): garantiza integridad referencial. El CASCADE cubre el borrado
-- físico de la fila de projects; como DeleteProject es SOFT-delete (deleted_at), la
-- revocación "el deploy-key muere con su proyecto" la ejecuta DeleteProject con un
-- DELETE explícito de api_tokens en la misma tx. El aislamiento por-tenant NO se
-- delega a la FK (que es global por id): lo garantiza la validación en el service
-- (GetProject bajo RLS: el proyecto debe existir en el tenant activo) + la RLS de
-- api_tokens (que sigue confinando cada fila a su tenant). La FK no relaja la RLS.
ALTER TABLE api_tokens
    ADD COLUMN project_id uuid REFERENCES projects(id) ON DELETE CASCADE;

-- Lookup por proyecto (p.ej. listar/revocar deploy-keys de un proyecto). Parcial:
-- solo indexa los tokens confinados (la inmensa mayoría son tenant-wide = NULL).
CREATE INDEX api_tokens_project_idx ON api_tokens (project_id) WHERE project_id IS NOT NULL;
