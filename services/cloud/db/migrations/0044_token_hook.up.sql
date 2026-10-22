-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Scope `hook`: la credencial de una URL de despliegue (/hooks/deploy/{token}).
-- Hasta ahora era un token `deploy` confinado al proyecto, y ese mismo token como
-- Bearer cambiaba el repositorio y el comando de release. La URL se pega en un CMS
-- ajeno y pasa por los logs del borde: filtrarla no puede dar la API (auditoría
-- 2026-10-06, CLOUD-17). Un token `hook` solo vale en esa ruta.
ALTER TABLE api_tokens DROP CONSTRAINT api_tokens_scope_check;
ALTER TABLE api_tokens ADD CONSTRAINT api_tokens_scope_check
    CHECK (scope IN ('read', 'write', 'deploy', 'hook'));
