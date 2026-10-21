-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Puerto de escucha y ruta de health check POR PROYECTO.
--
-- Hasta ahora ambos eran constantes del código (`appPort = 8080` en
-- controller/deploy.go y probes `HTTP GET /`), heredadas de las plantillas de
-- Dockerfile que genera el builder. Cualquier app que escuche en otro puerto —o
-- que no responda 2xx en «/»— nunca llega a `AvailableReplicas`, así que se queda
-- en `deploying` PARA SIEMPRE, sin un mensaje que lo explique. Eso descarta de
-- entrada las imágenes traídas de fuera (3000 de Next, 5000 de Flask, 8000 de
-- Django…) y es un muro de adopción, no una preferencia estética.
--
-- Los DEFAULT preservan el comportamiento exacto de antes de esta migración, así
-- que ningún proyecto existente cambia de conducta al aplicarla.
--
-- Rangos: puerto 1..65535 y ruta que empiece por «/» (el CHECK evita que una
-- entrada mal formada llegue al Deployment de Kubernetes, donde el error sería
-- opaco). La ruta vacía NO se permite: para desactivar la sonda se usará un campo
-- aparte el día que haga falta (hoy toda app de cliente es HTTP).

ALTER TABLE projects
    ADD COLUMN app_port    integer NOT NULL DEFAULT 8080,
    ADD COLUMN health_path text    NOT NULL DEFAULT '/';

ALTER TABLE projects
    ADD CONSTRAINT projects_app_port_range CHECK (app_port BETWEEN 1 AND 65535),
    ADD CONSTRAINT projects_health_path_abs CHECK (health_path LIKE '/%' AND length(health_path) <= 512);
