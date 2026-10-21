-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Pausar un proyecto: apagarlo sin borrarlo.
--
-- Un entorno de pruebas que solo se usa en horario de oficina, una demo que
-- terminó, un proyecto en espera de decisión: hasta ahora las dos únicas
-- opciones eran pagarlo mientras corría o BORRARLO y perder dominio, variables y
-- servicios. Es lo que Coolify llama pausar y DigitalOcean «destroy» a medias.
--
-- Pausado significa: cero réplicas en el clúster y CERO facturación de cómputo
-- (la consulta de metering lo excluye). Todo lo demás —dominio, TLS, variables,
-- base de datos, historial— se queda intacto y reanudar es un clic.
ALTER TABLE projects ADD COLUMN IF NOT EXISTS paused boolean NOT NULL DEFAULT false;

-- El metering recorre las producciones ready una y otra vez; sin este índice
-- parcial, filtrar por `paused` obliga a mirar filas que nunca van a facturar.
CREATE INDEX IF NOT EXISTS projects_activos_idx ON projects (tenant)
    WHERE deleted_at IS NULL AND NOT paused;
