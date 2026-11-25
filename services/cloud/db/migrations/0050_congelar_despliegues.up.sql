-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Congelar los despliegues de PRODUCCIÓN de un proyecto (DevX).
--
-- Es el «deploy freeze» de GitLab y la ventana de cambio de cualquier equipo de
-- operaciones: durante un cierre contable, una campaña o la noche de un viernes,
-- un push a `main` no puede llegar a producción por accidente. Hasta ahora la
-- única forma de pararlo era desconectar el repositorio, que también apaga las
-- previews y obliga a recordar volver a conectarlo.
--
-- Tabla APARTE y no tres columnas en `projects`, a propósito: `projectCols` se
-- escanea en una docena de consultas (algunas juntan proyecto y deployment), y
-- cada columna nueva ahí ha sido históricamente una consulta que se queda atrás
-- y revienta en ejecución (ver store/columnas_test.go). Una fila por proyecto
-- congelado; descongelar es borrarla, así que «no hay fila» es el estado normal.
--
-- Lo que NO congela, y por qué:
--   - las previews: revisar una rama no cambia lo que ve nadie fuera del equipo;
--   - el rollback: es la salida de emergencia, y una congelación que impide
--     volver atrás convierte un despliegue malo en un incidente sin vía de escape.
CREATE TABLE deploy_freezes (
    project_id uuid        PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    tenant     text        NOT NULL,
    -- El porqué, para quien intente desplegar y se lo encuentre. Acotado en la
    -- base como última red: el servicio ya rechaza antes un texto más largo.
    reason     text        NOT NULL DEFAULT '' CHECK (char_length(reason) <= 280),
    frozen_by  text        NOT NULL DEFAULT '',
    frozen_at  timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE deploy_freezes ENABLE ROW LEVEL SECURITY;
ALTER TABLE deploy_freezes FORCE ROW LEVEL SECURITY;

CREATE POLICY deploy_freezes_tenant ON deploy_freezes
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Política de FUSIÓN de organizaciones desde el principio, como pide el gate de
-- migcheck: bajo FORCE ROW LEVEL SECURITY el motor de fusión no vería estas
-- filas y el preflight abortaría la fusión entera (lo que le pasó a la 0033).
CREATE POLICY deploy_freezes_platform_merge ON deploy_freezes
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
