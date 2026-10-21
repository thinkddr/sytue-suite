-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Muestras de uso REAL de las apps de cliente (CPU y memoria observadas), que es
-- lo que convierte «te sobra tamaño» en una afirmación y no en una corazonada.
--
-- Hasta ahora el medidor registraba réplicas × memoria CONFIGURADA × horas: se
-- factura por lo que el cliente PIDIÓ, no por lo que gasta, así que no existía ni
-- un dato con el que recomendar nada.
--
-- Tres decisiones de forma que conviene entender antes de tocar la tabla:
--
--  1. La clave es el PROYECTO, no el despliegue. El tamaño de instancia es una
--     propiedad del proyecto (projects.instance_size), y cada push crea un
--     deployment nuevo: si se agrupara por deployment, la ventana de observación
--     se reiniciaría en cada despliegue y jamás se acumularían los tres días que
--     el motor exige para proponer una bajada.
--  2. Una fila por CUBO de 15 minutos, no por muestra. Acota el volumen a 96
--     filas por proyecto y día, y —con la clave primaria— hace el muestreo
--     idempotente: dos réplicas del controlador muestreando a la vez no duplican.
--  3. Se guarda el MÁXIMO entre los pods del proyecto en ese cubo, no la media.
--     El límite de memoria se aplica POR POD: el que más consume es el que muere
--     por OOM, así que promediar escondería justo el pod que importa.
CREATE TABLE resource_samples (
    tenant       text        NOT NULL,
    project_id   uuid        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    -- Inicio del cubo de 15 min (UTC).
    bucket       timestamptz NOT NULL,
    -- Máximos observados entre los pods de producción del proyecto en el cubo.
    cpu_milli    integer     NOT NULL CHECK (cpu_milli >= 0),
    memory_bytes bigint      NOT NULL CHECK (memory_bytes >= 0),
    PRIMARY KEY (project_id, bucket)
);

-- Lectura típica: la ventana reciente de un proyecto, de más nuevo a más viejo.
CREATE INDEX resource_samples_ventana ON resource_samples (tenant, project_id, bucket DESC);
-- Purga por antigüedad, cross-tenant.
CREATE INDEX resource_samples_purga ON resource_samples (bucket);

ALTER TABLE resource_samples ENABLE ROW LEVEL SECURITY;
ALTER TABLE resource_samples FORCE ROW LEVEL SECURITY;

CREATE POLICY resource_samples_tenant ON resource_samples
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Política de mantenimiento: el muestreador recorre TODOS los inquilinos en un
-- solo proceso, así que no puede ir bajo la política por-tenant. Mismo criterio
-- EXACTO que la 0002/0003 (`app.maintenance='on'`, que solo fija
-- pg.InMaintenanceTx): un flag explícito, y no «sin tenant fijado», que abriría
-- la tabla a cualquier transacción que se olvidara de ponerlo.
CREATE POLICY resource_samples_maintenance ON resource_samples
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
