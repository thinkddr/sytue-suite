-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Comando de release: lo que hay que ejecutar ANTES de que arranque la versión
-- nueva. En la práctica, casi siempre: migrar la base de datos.
--
-- Sin esto, una app con esquema versionado no se puede desplegar bien en Sytue
-- Cloud. Las dos salidas que quedaban son las dos malas:
--
--   1. Migrar al arrancar el contenedor. Con más de una réplica, TODAS migran a
--      la vez sobre la misma base; y si la migración falla, el pod entra en
--      CrashLoopBackOff y el despliegue muere sin decir por qué.
--   2. Migrar a mano por la terminal. Es decir, un paso manual en cada
--      despliegue, que es justo lo que un PaaS existe para quitar.
--
-- Se guarda en el PROYECTO y no en el deployment, igual que la escala y el
-- contrato de la app: es configuración, no una propiedad de la build. Así un
-- rollback no revive el comando de release de la versión antigua.
--
-- Cadena vacía = no hay comando, que es el estado de todos los proyectos
-- existentes: nada cambia para nadie al aplicar esta migración.
ALTER TABLE projects ADD COLUMN IF NOT EXISTS release_command text NOT NULL DEFAULT '';

-- Tope de longitud en la BASE y no solo en el servicio: el comando acaba en el
-- `args` de un contenedor, y un valor absurdo ahí lo rechaza el API server de
-- Kubernetes cuando ya es tarde (deployment a medias). Mejor un error claro al
-- guardarlo.
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_release_command_len;
ALTER TABLE projects ADD CONSTRAINT projects_release_command_len
    CHECK (length(release_command) <= 2000);
