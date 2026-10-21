-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Calidades elegidas por quien sube (domain/calidades.go). Se aplican a las dos
-- escaleras a la vez —HLS para la web, WebM para el escritorio—, así que se
-- guarda qué se eligió y qué existe ya en cada una:
--
--   qualities       lo elegido (alturas, de mayor a menor); NULL = lo recomendado,
--                   todas las que el original permita.
--   hls_qualities   las variantes del HLS que hay; NULL = un vídeo de antes, que
--                   tiene las que su original permitía.
--   webm_qualities  los WebM que hay (webm/<q>p.webm); NULL = uno de antes, que
--                   solo tiene webm/720p.webm.
--
-- Cambiar la elección encola un trabajo `qualities` que genera lo que falta y
-- borra lo que sobra; el mismo trabajo da la escalera WebM entera a los vídeos
-- de antes (cmd/cast encolar-calidades).

ALTER TABLE videos
    ADD COLUMN IF NOT EXISTS qualities      integer[] CHECK (qualities IS NULL OR cardinality(qualities) BETWEEN 1 AND 4),
    ADD COLUMN IF NOT EXISTS hls_qualities  integer[],
    ADD COLUMN IF NOT EXISTS webm_qualities integer[];

ALTER TABLE transcode_jobs DROP CONSTRAINT IF EXISTS transcode_jobs_kind_check;
ALTER TABLE transcode_jobs ADD CONSTRAINT transcode_jobs_kind_check
    CHECK (kind IN ('upload', 'live_vod', 'poster', 'qualities'));

-- Encolar desde mantenimiento: el trabajo de calidades que se vuelve a pedir al
-- terminar (la elección cambió mientras se cocía) y `encolar-calidades`, que
-- recorre todas las organizaciones.
CREATE POLICY transcode_jobs_mantenimiento_alta ON transcode_jobs
    FOR INSERT
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
