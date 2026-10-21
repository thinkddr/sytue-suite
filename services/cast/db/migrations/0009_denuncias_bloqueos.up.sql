-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Denuncia y bloqueo en Sytue Cast (docs/decisions.md, «Cast: denuncia y bloqueo»). Google Play exige las dos cosas a
-- una app cuyo contenido es de sus usuarios, y Cast además tiene parte pública.
--
-- Cuatro tablas con organización (`tenant` = la del CONTENIDO denunciado, no la de
-- quien denuncia: como los comentarios de 0003) y una sin ella, `person_blocks`,
-- que es de la persona y la sigue a cualquier organización.

-- ---------------------------------------------------------------------------
-- Denuncias. `kind` dice qué se denuncia y `target_id` cuál: el id del vídeo, del
-- comentario o del mensaje, el sub de la persona o el slug del canal.
-- `target_sub`, `target_name` y `excerpt` son la foto del momento: el chat se
-- retira a los 30 días y un comentario se puede borrar, y la cola tiene que seguir
-- diciendo qué se denunció.
--
-- `reporter_sub` (vacío sin sesión) NO sale por la API: se guarda para la
-- auditoría. `reporter_key` es la clave de deduplicación: el sub, o un HMAC de la
-- IP para quien denuncia sin cuenta.
-- ---------------------------------------------------------------------------
CREATE TABLE reports (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    kind            text        NOT NULL CHECK (kind IN ('video', 'comment', 'chat', 'person', 'channel')),
    video_id        uuid        REFERENCES videos (id) ON DELETE CASCADE,
    target_id       text        NOT NULL CHECK (target_id <> ''),
    target_sub      text        NOT NULL DEFAULT '',
    target_name     text        NOT NULL DEFAULT '',
    excerpt         text        NOT NULL DEFAULT '' CHECK (length(excerpt) <= 300),
    reason          text        NOT NULL CHECK (reason IN ('harassment', 'sexual', 'violence', 'spam', 'copyright', 'other')),
    details         text        NOT NULL DEFAULT '' CHECK (length(details) <= 1000),
    reporter_sub    text        NOT NULL DEFAULT '',
    reporter_tenant text        NOT NULL DEFAULT '',
    reporter_key    text        NOT NULL CHECK (reporter_key <> ''),
    status          text        NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'dismissed', 'actioned')),
    resolution      text        NOT NULL DEFAULT '',
    resolved_by     text        NOT NULL DEFAULT '',
    resolved_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- Deduplicación: una denuncia ABIERTA por persona y objetivo. Resuelta, la misma
-- persona puede volver a denunciar si el problema vuelve.
CREATE UNIQUE INDEX reports_una_abierta ON reports (tenant, kind, target_id, reporter_key) WHERE status = 'open';
CREATE INDEX reports_cola ON reports (tenant, status, created_at DESC, id);

-- ---------------------------------------------------------------------------
-- Auditoría de la moderación: cada denuncia y cada acción sobre ella, y los
-- bloqueos de organización. Las columnas son las de `auditlog.Entrada`.
-- ---------------------------------------------------------------------------
CREATE TABLE moderation_audit (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant      text        NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    actor       text        NOT NULL,
    action      text        NOT NULL,
    target      text        NOT NULL DEFAULT '',
    detail      jsonb       NOT NULL DEFAULT '{}'
);

CREATE INDEX moderation_audit_reciente ON moderation_audit (tenant, occurred_at DESC, id DESC);

-- ---------------------------------------------------------------------------
-- Quién modera cada organización, hasta donde Cast lo sabe: las personas con rol
-- owner o admin que han abierto Cast. El Hub entrega los avisos por persona y Cast
-- no tiene la lista de miembros, así que avisa a quienes ha visto.
-- ---------------------------------------------------------------------------
CREATE TABLE moderators (
    tenant  text        NOT NULL,
    sub     text        NOT NULL,
    seen_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, sub)
);

-- ---------------------------------------------------------------------------
-- Bloqueo de organización: esa persona no comenta ni escribe en el chat de ningún
-- vídeo de la organización. Lo ponen y lo quitan quienes la administran.
-- ---------------------------------------------------------------------------
CREATE TABLE org_blocks (
    tenant       text        NOT NULL,
    blocked_sub  text        NOT NULL CHECK (blocked_sub <> ''),
    blocked_name text        NOT NULL DEFAULT '',
    created_by   text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, blocked_sub)
);

-- ---------------------------------------------------------------------------
-- Bloqueo personal: quien bloquea deja de ver los comentarios y mensajes de la otra
-- persona y de recibir sus avisos, y la otra persona deja de poder escribir en los
-- vídeos de quien la bloqueó. No lleva organización: es de la persona.
-- ---------------------------------------------------------------------------
CREATE TABLE person_blocks (
    blocker_sub  text        NOT NULL CHECK (blocker_sub <> ''),
    blocked_sub  text        NOT NULL CHECK (blocked_sub <> ''),
    blocked_name text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_sub, blocked_sub),
    CHECK (blocker_sub <> blocked_sub)
);

CREATE INDEX person_blocks_bloqueado ON person_blocks (blocked_sub);

-- ---------------------------------------------------------------------------
-- Aislamiento.
-- ---------------------------------------------------------------------------
ALTER TABLE reports          ENABLE ROW LEVEL SECURITY;
ALTER TABLE reports          FORCE ROW LEVEL SECURITY;
ALTER TABLE moderation_audit ENABLE ROW LEVEL SECURITY;
ALTER TABLE moderation_audit FORCE ROW LEVEL SECURITY;
ALTER TABLE moderators       ENABLE ROW LEVEL SECURITY;
ALTER TABLE moderators       FORCE ROW LEVEL SECURITY;
ALTER TABLE org_blocks       ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_blocks       FORCE ROW LEVEL SECURITY;
ALTER TABLE person_blocks    ENABLE ROW LEVEL SECURITY;
ALTER TABLE person_blocks    FORCE ROW LEVEL SECURITY;

CREATE POLICY reports_tenant_isolation ON reports
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY moderation_audit_tenant_isolation ON moderation_audit
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY moderators_tenant_isolation ON moderators
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY org_blocks_tenant_isolation ON org_blocks
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY reports_platform_merge ON reports
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY moderation_audit_platform_merge ON moderation_audit
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY moderators_platform_merge ON moderators
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY org_blocks_platform_merge ON org_blocks
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

-- El bloqueo personal se lee con `app.cast_persona` = el sub de quien pregunta, y
-- lo ven sus dos extremos: quien bloquea (su lista) y quien está bloqueado (el
-- servidor tiene que saber, al escribir esa persona, que no puede). Darlo de alta
-- y quitarlo, solo quien bloquea.
CREATE POLICY person_blocks_lectura ON person_blocks
    FOR SELECT
    USING (current_setting('app.cast_persona', true) IN (blocker_sub, blocked_sub));

CREATE POLICY person_blocks_alta ON person_blocks
    FOR INSERT
    WITH CHECK (blocker_sub = current_setting('app.cast_persona', true));

CREATE POLICY person_blocks_baja ON person_blocks
    FOR DELETE
    USING (blocker_sub = current_setting('app.cast_persona', true));
