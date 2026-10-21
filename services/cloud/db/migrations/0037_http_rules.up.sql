-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Redirecciones y cabeceras del proyecto, aplicadas en el BORDE.
--
-- Es lo que en Vercel se escribe en `vercel.json` y en Netlify en `_redirects`:
-- mover una URL que cambió sin perder los enlaces de nadie, y poner las cabeceras
-- que la aplicación no pone. Sin esto la única salida era resolverlo dentro del
-- código de cada app — y en un sitio estático, ni eso.
--
-- Un solo jsonb y no dos tablas: son como mucho veinte redirecciones y veinte
-- cabeceras por proyecto, se leen y se escriben SIEMPRE juntas (el reconciliador
-- las materializa de una vez) y nadie va a consultarlas por separado. Dos tablas
-- con sus claves ajenas serían más SQL para exactamente el mismo comportamiento.
ALTER TABLE projects ADD COLUMN IF NOT EXISTS http_rules jsonb NOT NULL DEFAULT '{}'::jsonb;

-- Tope defensivo: el contenido ya lo valida el dominio (20 y 20), pero la columna
-- acaba en objetos del clúster y un jsonb desmedido no debe llegar tan lejos.
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_http_rules_len;
ALTER TABLE projects ADD CONSTRAINT projects_http_rules_len
    CHECK (length(http_rules::text) <= 20000);
