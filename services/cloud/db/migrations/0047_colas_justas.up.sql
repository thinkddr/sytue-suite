-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)

-- Colas compartidas sin atascos y dominios que no se pueden ocupar
-- (auditoría 2026-10-06, CLOUD-06 y CLOUD-10).

-- checked_at: cuándo miró el controlador por última vez. Las colas se recorren
-- por «el que lleva más tiempo sin mirarse», no por antigüedad: antes 50
-- dominios o servicios que nunca avanzan, creados antes que los demás, paraban
-- la verificación o el aprovisionamiento de todas las organizaciones.
ALTER TABLE domains ADD COLUMN checked_at timestamptz;
ALTER TABLE managed_services ADD COLUMN checked_at timestamptz;

-- Un hostname solo es de un proyecto cuando está VERIFICADO. Antes era único en
-- cualquier estado: otra organización añadía `tienda.victima.com`, no lo
-- verificaba nunca y la dueña ya no podía añadirlo. Ahora las reclamaciones
-- pendientes conviven, gana quien publica su TXT y las pendientes caducan a los
-- siete días (store.ListPendingDomains).
DROP INDEX domains_unique_hostname;
CREATE UNIQUE INDEX domains_unique_hostname_verificado
    ON domains (lower(hostname)) WHERE status IN ('verified', 'active');
CREATE UNIQUE INDEX domains_unique_hostname_proyecto
    ON domains (project_id, lower(hostname));
