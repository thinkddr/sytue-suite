-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Seguimiento de hilos (thread follows) — Ola 9. Un usuario SIGUE un hilo (su
-- mensaje raíz) y la vista global «Hilos» le lista sus hilos seguidos con el número
-- de respuestas NO leídas por hilo. Auto-follow al responder: el que responde y el
-- autor de la raíz (ver store.autoFollowOnReply).
--
-- last_viewed_at marca hasta cuándo vio el usuario el hilo; el contador de no leídos
-- son las respuestas con created_at > last_viewed_at de otros autores.
--
-- last_activity se DENORMALIZA (= created_at de la última respuesta) para ordenar la
-- vista global por keyset indexado sin recomputar un max() por hilo en cada consulta.
-- Se bombea al publicar una respuesta con un UPDATE acotado a los SEGUIDORES del hilo
-- (sus participantes), no a los N miembros del canal: sin fan-out de N filas por
-- mensaje, igual criterio que mentions_all (mig. 0003).

CREATE TABLE thread_follows (
    tenant         text        NOT NULL,
    root_id        uuid        NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    subject        text        NOT NULL,
    last_viewed_at timestamptz NOT NULL DEFAULT now(),
    last_activity  timestamptz NOT NULL DEFAULT now(),
    created_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, root_id, subject)
);

-- Keyset de la vista global «Hilos» por usuario: cubre el WHERE (tenant, subject) y
-- el ORDER BY (last_activity DESC, root_id DESC) de ListFollowedThreads — coste
-- O(página), no O(hilos seguidos). Escala a cientos de miles. El bombeo por reply
-- (UPDATE ... WHERE tenant, root_id) lo cubre el prefijo de la PRIMARY KEY.
CREATE INDEX thread_follows_by_subject
    ON thread_follows (tenant, subject, last_activity DESC, root_id DESC);

-- RLS deny-by-default por tenant (mismo patrón que el resto del esquema).
ALTER TABLE thread_follows ENABLE ROW LEVEL SECURITY;
ALTER TABLE thread_follows FORCE ROW LEVEL SECURITY;
CREATE POLICY thread_follows_tenant_isolation ON thread_follows
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
