-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)

-- Cuándo se borraron las imágenes de un proyecto borrado (CLOUD-08).
--
-- El slug de un proyecto borrado quedaba libre en el acto, y quien lo tomaba
-- heredaba `<slug>.apps…` y el repositorio de imágenes `<registry>/<slug>` del
-- dueño anterior. Decisión del dueño (2026-10-17): el slug vuelve a estar libre
-- a los 90 días y después de borrar sus imágenes, dominios y certificados; antes
-- de ese plazo solo la misma organización lo recupera. El controlador borra las
-- imágenes y anota aquí cuándo; mientras sea NULL, el slug sigue retenido.
ALTER TABLE projects ADD COLUMN purged_at timestamptz;

CREATE INDEX projects_slug_retenido_idx ON projects (slug)
    WHERE deleted_at IS NOT NULL;
