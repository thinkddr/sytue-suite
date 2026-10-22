-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- «Presentar con Cast» desde Sytue Diapos: un directo puede llevar una presentación,
-- con la diapositiva en la que va y, por cada cambio, una marca de tiempo que luego
-- sirve de capítulo en la grabación.
--
-- Solo se guarda la POSICIÓN (número y título de la diapositiva): ni el contenido de
-- la presentación ni las notas del ponente salen nunca de Diapos.
--
-- Las dos tablas cuelgan del vídeo y viven en SU organización (`tenant` es el del
-- vídeo, no el de quien mira). Quién las lee lo decide `internal/acceso` antes de
-- llegar aquí; la RLS solo separa organizaciones.

CREATE TABLE live_presentations (
    video_id     uuid        PRIMARY KEY REFERENCES videos (id) ON DELETE CASCADE,
    tenant       text        NOT NULL,
    -- De dónde sale, p. ej. `docs:<id>`; vacío si no se dice.
    source_ref   text        NOT NULL DEFAULT '' CHECK (length(source_ref) <= 200),
    total        integer     NOT NULL CHECK (total BETWEEN 1 AND 2000),
    slide_index  integer     NOT NULL CHECK (slide_index >= 0),
    slide_title  text        NOT NULL DEFAULT '' CHECK (length(slide_title) <= 400),
    -- Quién la lanzó y cuándo: la huella de auditoría de la presentación.
    started_by   text        NOT NULL,
    started_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CHECK (slide_index < total)
);

-- Un capítulo por cada cambio de diapositiva, en el instante (ms desde que el
-- directo empezó) en que se pasó. Hasta 2000 por vídeo.
CREATE TABLE slide_marks (
    video_id  uuid    NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    tenant    text    NOT NULL,
    seq       integer NOT NULL CHECK (seq BETWEEN 1 AND 2000),
    at_ms     bigint  NOT NULL CHECK (at_ms >= 0),
    idx       integer NOT NULL CHECK (idx >= 0),
    title     text    NOT NULL DEFAULT '' CHECK (length(title) <= 400),
    PRIMARY KEY (video_id, seq)
);

ALTER TABLE live_presentations ENABLE ROW LEVEL SECURITY;
ALTER TABLE live_presentations FORCE ROW LEVEL SECURITY;
ALTER TABLE slide_marks        ENABLE ROW LEVEL SECURITY;
ALTER TABLE slide_marks        FORCE ROW LEVEL SECURITY;

CREATE POLICY live_presentations_tenant_isolation ON live_presentations
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY slide_marks_tenant_isolation ON slide_marks
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY live_presentations_platform_merge ON live_presentations
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY slide_marks_platform_merge ON slide_marks
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
