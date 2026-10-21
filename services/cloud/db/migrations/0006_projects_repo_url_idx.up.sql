-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Ola B — índice funcional para ResolveRepo (ruta caliente de CADA webhook git).
-- ResolveRepo (store/webhooks.go) filtra por lower(repo_url)=lower($1) AND
-- deleted_at IS NULL; sin índice sobre la expresión hace un Seq Scan de `projects`
-- en cada push. A escala (cientos de miles de proyectos, cubo del webhook a 50/s)
-- eso son hasta 50 Seq Scans/seg de la tabla entera. El índice parcial sobre
-- lower(repo_url) lo convierte en index scan y permite subir con seguridad el
-- techo del rate-limit de webhooks. Espeja projects_tenant_idx (0001), también
-- parcial WHERE deleted_at IS NULL.

CREATE INDEX projects_repo_url_idx ON projects (lower(repo_url)) WHERE deleted_at IS NULL;
