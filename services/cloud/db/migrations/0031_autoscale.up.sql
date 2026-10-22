-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Techo y objetivo del autoescalado POR PROYECTO.
--
-- Desde que existe el HPA, sus dos parámetros estaban escritos a mano en el
-- controlador: objetivo de CPU al 75 % y máximo = 5× las instancias configuradas
-- (con suelo 3). El mínimo sí se podía configurar (0023, instance_count), pero el
-- techo NO: una app con 1 instancia no podía pasar de 3 pods por mucha carga que
-- le llegara, y la única forma de levantar el techo era subir el MÍNIMO —que se
-- paga las 24 horas, tenga o no tenga tráfico. Es decir: para aguantar un pico se
-- cobraba el pico entero todo el mes.
--
-- El objetivo de CPU tampoco es una constante universal: al 75 % una app sensible
-- a la latencia ya va cargada cuando el HPA empieza a reaccionar, y una app de
-- procesamiento por lotes desperdicia la cuarta parte de lo que paga. DO App
-- Platform y Scaleway exponen ambos; aquí eran inalcanzables incluso por API.
--
-- Los DEFAULT preservan EXACTAMENTE el comportamiento anterior:
--   * autoscale_max_count = 0 significa «automático», que es la regla histórica
--     (5× las instancias, mínimo 3). No hay constante que la reproduzca, porque
--     depende de instance_count; de ahí el centinela en vez de un número fijo.
--   * autoscale_cpu_target = 75 es el valor que estaba escrito en el código.
-- Ningún proyecto existente cambia de conducta ni de factura al aplicarla.

ALTER TABLE projects
    ADD COLUMN autoscale_max_count  integer  NOT NULL DEFAULT 0,
    ADD COLUMN autoscale_cpu_target smallint NOT NULL DEFAULT 75;

-- El rango se acota aquí como última red (el 400 explicativo lo da la API). El
-- techo de 50 es el mismo MaxInstanceCount de 0023: el límite REAL lo pone la
-- ResourceQuota del namespace según el plan, esto solo evita que un número
-- absurdo llegue a Kubernetes, donde el error sería opaco.
--
-- El suelo del objetivo de CPU es 10 %: por debajo el HPA escala con ruido de
-- medición y una app ociosa mantendría el máximo de pods encendidos —una factura
-- inesperada con forma de error de configuración.
ALTER TABLE projects
    ADD CONSTRAINT projects_autoscale_max_range
        CHECK (autoscale_max_count BETWEEN 0 AND 50),
    ADD CONSTRAINT projects_autoscale_cpu_range
        CHECK (autoscale_cpu_target BETWEEN 10 AND 100);
