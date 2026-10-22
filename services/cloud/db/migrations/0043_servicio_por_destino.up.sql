-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- A qué despliegues entrega sus credenciales un servicio gestionado: a los de
-- producción o a las vistas previas.
--
-- Hasta ahora las credenciales de TODOS los servicios del proyecto se inyectaban
-- en todo despliegue, también en las vistas previas de un pull request. Es decir:
-- el código de una rama sin revisar arrancaba con el DATABASE_URL de la base de
-- producción y las claves del almacén de producción (auditoría 2026-10-06,
-- CLOUD-01). Una vista previa tiene que traer lo suyo, no heredar lo de
-- producción.
--
-- Es una COLUMNA y no una clave de `config_json` a propósito: `config_json` lo
-- reescribe entero el aprovisionamiento cada vez que termina (MarkServiceReady) y
-- además lo puede tocar el cliente por PATCH. Un dato del que depende qué
-- despliegue recibe la contraseña de una base de datos no puede vivir donde otro
-- lo pisa.
--
-- 'production' por defecto: es lo que eran todos los servicios existentes, así
-- que producción sigue recibiendo exactamente lo mismo que recibía. Lo que cambia
-- es que las vistas previas dejan de recibirlo.
ALTER TABLE managed_services
    ADD COLUMN IF NOT EXISTS target text NOT NULL DEFAULT 'production';

ALTER TABLE managed_services DROP CONSTRAINT IF EXISTS managed_services_target_check;
ALTER TABLE managed_services ADD CONSTRAINT managed_services_target_check
    CHECK (target IN ('production', 'preview'));

-- Las vistas previas que ya están en marcha arrancaron con las reglas de antes:
-- sus contenedores llevan en el entorno los secretos y la base de datos de
-- producción, y entre ellas puede haber alguna abierta desde un fork. Cambiar el
-- Secret no reinicia un pod, así que la única forma de que dejen de tenerlos es
-- retirarlas. El controlador desmonta sus objetos en el siguiente ciclo y la
-- vista previa vuelve, ya sin nada de producción, con el siguiente push al pull
-- request (o no vuelve, si era de un fork).
UPDATE deployments
   SET status = 'canceled', updated_at = now()
 WHERE target = 'preview'
   AND status IN ('queued', 'building', 'deploying', 'ready');
