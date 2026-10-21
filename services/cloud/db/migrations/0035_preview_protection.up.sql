-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Previews cerradas con contraseña (Deployment Protection).
--
-- Una preview es la rama de alguien SIN REVISAR, con datos de prueba dentro y una
-- URL pública que cualquiera puede visitar y Google indexar. Hasta ahora salía a
-- internet igual de abierta que producción, y la única forma de que no lo
-- estuviera era no crearla.
--
-- Se guarda la LÍNEA htpasswd («preview:$2y$…»), que es lo que consume el
-- middleware de Traefik. Es un hash bcrypt, no un secreto reversible: la
-- contraseña en claro se enseña UNA vez al activarla y no se guarda en ninguna
-- parte, ni cifrada. Si se pierde, se genera otra.
--
-- Vacío = preview abierta, que es el estado de todos los proyectos existentes:
-- nada cambia para nadie al aplicar esta migración.
ALTER TABLE projects ADD COLUMN IF NOT EXISTS preview_auth_hash text NOT NULL DEFAULT '';

-- Tope defensivo: una línea htpasswd con bcrypt son ~70 caracteres. Un valor
-- desmedido acabaría en un Secret de Kubernetes, donde el rechazo llega tarde.
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_preview_auth_len;
ALTER TABLE projects ADD CONSTRAINT projects_preview_auth_len
    CHECK (length(preview_auth_hash) <= 200);
