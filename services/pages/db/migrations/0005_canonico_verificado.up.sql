-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- WEB-01 (auditoría de seguridad 2026-10-06). `sites.canonical_host` se escribía con
-- texto libre del navegador (PUT /sites/{id}): un sitio ponía ahí el host de otro y se
-- quedaba con él en el mapa de hosts y en la API pública. Desde este cambio solo lo
-- escribe el servidor, y solo con un dominio ACTIVO de ese mismo sitio.
--
-- Esta migración limpia lo que hubiera: todo canónico sin un dominio activo propio
-- detrás vuelve a NULL, que es `<slug>.<PAGES_SITES_DOMAIN>`. No se pierde nada
-- legítimo: un canónico legítimo ES un dominio activo del sitio.
--
-- `app.maintenance` solo dura esta transacción: sin él, con la RLS forzada y un rol que
-- no la elude, el UPDATE no vería ninguna fila y no limpiaría nada en silencio.
SELECT set_config('app.maintenance', 'on', true);

UPDATE sites s
   SET canonical_host = NULL
 WHERE s.canonical_host IS NOT NULL
   AND NOT EXISTS (
         SELECT 1 FROM domains d
          WHERE d.site_id = s.id AND d.tenant = s.tenant
            AND d.host = s.canonical_host AND d.status = 'active');

SELECT set_config('app.maintenance', '', true);
