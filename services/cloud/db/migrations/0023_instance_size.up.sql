-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Tamaño de instancia y número de instancias POR PROYECTO.
--
-- Hasta ahora toda app corría con los mismos recursos, escritos a mano en el
-- controlador (100m/128Mi de petición, 1 CPU/512Mi de límite), y el número de
-- réplicas existía en `deployments.replicas` pero NINGÚN endpoint ni el CLI lo
-- escribía: una app con carga real no se podía escalar.
--
-- El valor va en el PROYECTO y no en el deployment a propósito: el tamaño es
-- configuración de la app, no una propiedad de la build. Así, cambiarlo surte
-- efecto en la siguiente reconciliación sin tener que volver a desplegar, y un
-- rollback a un deployment antiguo NO revierte la escala —que es lo que un
-- operador espera cuando ha subido instancias porque hay carga.
--
-- Los DEFAULT reproducen el comportamiento exacto de antes ('basic' son justo los
-- recursos que estaban escritos a mano, y 1 instancia), así que ninguna app
-- existente cambia de conducta ni de factura al aplicar esta migración.

ALTER TABLE projects
    ADD COLUMN instance_size  text    NOT NULL DEFAULT 'basic',
    ADD COLUMN instance_count integer NOT NULL DEFAULT 1;

-- El catálogo de tamaños vive en el código (domain.InstanceSizes) porque de ahí
-- salen también los recursos concretos; aquí solo se acota el rango del contador,
-- que es lo que evita que un número absurdo llegue a Kubernetes con un error opaco.
ALTER TABLE projects
    ADD CONSTRAINT projects_instance_count_range CHECK (instance_count BETWEEN 1 AND 50);
