-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Servicio 'worker': un proceso PERMANENTE sin puerto.
--
-- Es lo que en DigitalOcean App Platform se llama «worker component» y en Heroku
-- un dyno de tipo worker: el consumidor de una cola, el que procesa vídeo, el que
-- manda correos en segundo plano. No sirve HTTP, así que ni Service ni Ingress.
--
-- Lo que había cubría los otros tres casos y no este: `functions` es HTTP con su
-- dirección pública, `cron` corre y termina cada X, `job` corre y termina una vez.
-- Un consumidor de RabbitMQ no encaja en ninguno — la única forma de tenerlo era
-- disfrazarlo de función y dejar un puerto abierto que nadie usa.
-- Y DE PASO, un fallo que llevaba ahí desde que existe el catálogo de apps: el
-- CHECK vigente es el de la 0012 y NO admite 'app'. La capa de dominio sí lo
-- acepta (ValidServiceKind), así que la petición pasaba toda la validación y
-- moría en la base con una violación de restricción — o sea, levantar cualquier
-- app del catálogo era imposible en producción y el error no señalaba a la causa.
ALTER TABLE managed_services DROP CONSTRAINT IF EXISTS managed_services_kind_check;
ALTER TABLE managed_services ADD CONSTRAINT managed_services_kind_check
    CHECK (kind IN ('postgres', 'storage', 'functions', 'cron', 'job', 'worker', 'auth', 'app'));
