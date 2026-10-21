-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Avisos y errores de los servicios, para poder ENSEÑARLOS.
--
-- Hasta ahora los servicios escribían JSON a stdout y ahí moría: sin agregador,
-- saber qué está fallando en producción exigía una terminal contra el clúster, y
-- los logs de un pod reiniciado ya no existen. La pregunta «¿qué está roto?»
-- solo la podía responder quien tuviera kubeconfig, y solo a tiempo.
--
-- Solo warn y error. El info es el 99 % del volumen y no responde esa pregunta:
-- guardarlo sería pagar almacenamiento para no leerlo nunca.
--
-- Sin clave ajena a `users` ni a `organizations` a propósito: esto es
-- diagnóstico de la PLATAFORMA, no del negocio, y un registro que se borra en
-- cascada al dar de baja algo es justo el que hará falta para entender por qué
-- se dio de baja.

CREATE TABLE service_logs (
    id          bigserial PRIMARY KEY,
    occurred_at timestamptz NOT NULL,
    -- 'WARN' | 'ERROR' tal como los nombra slog. Sin CHECK cerrado: si algún día
    -- se capturara otro nivel, es preferible guardarlo a rechazar la escritura y
    -- perder el registro por una validación de catálogo.
    level       text NOT NULL,
    service     text NOT NULL,
    message     text NOT NULL,
    -- Atributos ya redactados y aplanados a texto por el capturador.
    attrs       jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- La consulta que hace la pantalla es «lo último, filtrado por nivel»: el índice
-- descendente la sirve sin ordenar y sin recorrer la tabla entera.
CREATE INDEX service_logs_occurred_desc_idx ON service_logs (occurred_at DESC, id DESC);
-- Y el agrupado por servicio y mensaje, que es lo que convierte mil líneas
-- iguales en una fila que dice «esto lleva pasando 1.000 veces».
CREATE INDEX service_logs_grupo_idx ON service_logs (service, message, occurred_at DESC);
