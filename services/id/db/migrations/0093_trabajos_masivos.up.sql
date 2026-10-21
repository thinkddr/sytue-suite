-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- TRABAJOS MASIVOS VISIBLES Y CANCELABLES.
--
-- `POST /admin/users/bulk` aplicaba una acción a doscientas personas y NO se
-- podía ni mirar ni parar: quien lo lanzaba se quedaba delante de una petición
-- abierta, sin saber por dónde iba, y si se había equivocado de selección la
-- única salida era cerrar la pestaña —lo que NO para nada: el servidor sigue—.
-- Suspender por error a medio departamento es exactamente el caso en el que
-- hace falta un botón de parar.
--
-- LO QUE ESTO ES Y LO QUE NO. El trabajo sigue ejecutándose DENTRO de la
-- petición (doscientas personas caben de sobra en su plazo): esto no es una
-- cola ni un worker. Lo que añade es que el progreso se ESCRIBE elemento a
-- elemento, así que otra petición puede verlo y puede pedir la cancelación; el
-- bucle la mira antes de cada persona y se detiene con lo hecho hecho y lo
-- demás sin tocar.
--
-- `latido` existe porque una petición puede morirse (un pod que se va, una red
-- que se corta) y dejar la fila en 'ejecutando' para siempre. Un trabajo sin
-- latido reciente se REPORTA como interrumpido: no se inventa que terminó.

CREATE TABLE bulk_jobs (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    accion        text        NOT NULL,
    total         integer     NOT NULL DEFAULT 0,
    hechos        integer     NOT NULL DEFAULT 0,
    estado        text        NOT NULL DEFAULT 'ejecutando',
    -- La cancelación se PIDE desde otra petición; el bucle la obedece. No se
    -- mata nada por la espalda: una acción a medias sin cerrar su fila sería
    -- peor que terminarla.
    cancelacion_pedida boolean NOT NULL DEFAULT false,
    -- El informe POR ELEMENTO, tal como lo devolvía la respuesta síncrona.
    resultados    jsonb       NOT NULL DEFAULT '[]'::jsonb,
    creado_por    uuid        REFERENCES users (id) ON DELETE SET NULL,
    creado_en     timestamptz NOT NULL DEFAULT now(),
    latido        timestamptz NOT NULL DEFAULT now(),
    terminado_en  timestamptz,

    CONSTRAINT bulk_jobs_estado CHECK (estado IN ('ejecutando', 'terminado', 'cancelado'))
);

-- El listado es «los de esta organización, los últimos primero».
CREATE INDEX bulk_jobs_de_org ON bulk_jobs (org_id, creado_en DESC);
