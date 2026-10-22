-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)

-- Host propio para las apps del catálogo: `<proyecto>-<app>.app.<dominio>`.
--
-- Era `<app>-<slug>.<dominio>`, la misma forma que la URL de producción de un
-- proyecto, así que otra organización podía crear el proyecto `<app>-<slug>` y
-- quedarse con parte del tráfico de esa app (auditoría 2026-10-06, CLOUD-03).
-- Se reescriben las existentes y vuelven a la cola de aprovisionamiento para que
-- el controlador rehaga su Ingress con el host nuevo.
--
-- La puerta de mantenimiento: managed_services lleva RLS forzada.
SELECT set_config('app.maintenance', 'on', false);

UPDATE managed_services
   SET config_json = jsonb_set(
           config_json, '{web_host}',
           to_jsonb(project_id::text || '-' || (config_json ->> 'app_id') || '.app.'
                    || substring(config_json ->> 'web_host' FROM position('.' IN config_json ->> 'web_host') + 1))),
       status = 'provisioning',
       updated_at = now()
 WHERE kind = 'app'
   AND deleted_at IS NULL
   AND coalesce(config_json ->> 'web_host', '') <> ''
   AND config_json ->> 'web_host' NOT LIKE '%.app.%';

SELECT set_config('app.maintenance', '', false);
