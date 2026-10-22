-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Vuelta atrás de las calidades: los trabajos `qualities` se retiran (el binario
-- anterior no los conoce) y los vídeos se quedan con lo que ya se generó en el
-- almacén; webm_key sigue apuntando a un WebM que existe.

DROP POLICY IF EXISTS transcode_jobs_mantenimiento_alta ON transcode_jobs;

DELETE FROM transcode_jobs WHERE kind = 'qualities';
ALTER TABLE transcode_jobs DROP CONSTRAINT IF EXISTS transcode_jobs_kind_check;
ALTER TABLE transcode_jobs ADD CONSTRAINT transcode_jobs_kind_check
    CHECK (kind IN ('upload', 'live_vod', 'poster'));

ALTER TABLE videos
    DROP COLUMN IF EXISTS webm_qualities,
    DROP COLUMN IF EXISTS hls_qualities,
    DROP COLUMN IF EXISTS qualities;
