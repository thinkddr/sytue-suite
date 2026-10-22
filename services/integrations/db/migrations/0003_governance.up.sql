-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- GOBERNANZA L5 (OLA 8): DLP/política en el egress + auditoría de decisiones. El
-- enforcement central que exige que TODA llamada a terceros esté sujeta a política
-- y sea imposible de esquivar (vive dentro del ÚNICO RoundTripper de internal/policy).
-- Dos tablas por-org (RLS deny-by-default, igual que 0001/0002):
--   - policies         : reglas que, ANTES de que un request salga, deciden
--                        allow|warn|block según METADATOS del egress (api_family =
--                        plantilla, host destino, método, prefijo de ruta) y, best
--                        effort, un patrón declarado sobre el cuerpo. La `condition`
--                        es jsonb opaco al SQL (lo compila el motor en Go).
--   - policy_decisions : registro append-only de las decisiones NO triviales
--                        (block/warn) — "qué regla bloqueó/marcó qué egress" — para
--                        auditar el enforcement. Un block NO deja fila en egress_audit
--                        (nunca salió): esta tabla es la única evidencia del bloqueo.
--
-- Mismo modelo de aislamiento que 0001: org_id (slug), RLS ENABLE+FORCE comparando
-- con current_setting('app.tenant', true), y el rol NO superusuario integrations_app.

-- ---------------------------------------------------------------------------
-- policies — reglas de política de egress por organización. `condition` es un
-- objeto JSON con campos opcionales AND-combinados (api_family, host, method,
-- path_prefix, body_contains); vacío = comodín (casa todo). `action` decide el
-- destino del request que casa. `enabled` retira una regla sin borrarla.
-- ---------------------------------------------------------------------------
CREATE TABLE policies (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     text        NOT NULL,
    name       text        NOT NULL,
    condition  jsonb       NOT NULL DEFAULT '{}'::jsonb,
    action     text        NOT NULL DEFAULT 'block'
                           CHECK (action IN ('allow', 'warn', 'block')),
    enabled    boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT policies_name_not_blank CHECK (length(name) > 0)
);

-- Carga por org de las reglas HABILITADAS en orden estable (created_at): el motor
-- las cachea por org y las evalúa en ese orden (primera regla decisiva gana).
CREATE INDEX policies_org_enabled ON policies (org_id, created_at) WHERE enabled;

-- ---------------------------------------------------------------------------
-- policy_decisions — bitácora append-only de decisiones block/warn. policy_id es
-- una referencia INFORMATIVA a la regla que casó (SIN FK: la bitácora sobrevive al
-- borrado de la regla); policy_name conserva el nombre en el momento de decidir.
-- request_id correlaciona con el X-Request-Id de la petición del usuario que originó
-- el egress. Solo METADATOS: jamás payloads ni credenciales.
-- ---------------------------------------------------------------------------
CREATE TABLE policy_decisions (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id      text        NOT NULL,
    instance_id uuid,
    policy_id   uuid,
    policy_name text        NOT NULL DEFAULT '',
    action      text        NOT NULL
                            CHECK (action IN ('warn', 'block')),
    request_id  text        NOT NULL DEFAULT '',
    host        text        NOT NULL DEFAULT '',
    method      text        NOT NULL DEFAULT '',
    occurred_at timestamptz NOT NULL DEFAULT now()
);

-- Sirve el visor (org, más reciente primero) Y el barrido de retención
-- (DELETE ... WHERE occurred_at < corte), ambos por org.
CREATE INDEX policy_decisions_org_occurred ON policy_decisions (org_id, occurred_at DESC);

-- ---------------------------------------------------------------------------
-- Row-Level Security: aislamiento por org deny-by-default (igual que 0001/0002).
-- ---------------------------------------------------------------------------
ALTER TABLE policies         ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_decisions ENABLE ROW LEVEL SECURITY;

ALTER TABLE policies         FORCE ROW LEVEL SECURITY;
ALTER TABLE policy_decisions FORCE ROW LEVEL SECURITY;

CREATE POLICY policies_org_isolation ON policies
    USING (org_id = current_setting('app.tenant', true))
    WITH CHECK (org_id = current_setting('app.tenant', true));
CREATE POLICY policy_decisions_org_isolation ON policy_decisions
    USING (org_id = current_setting('app.tenant', true))
    WITH CHECK (org_id = current_setting('app.tenant', true));

-- Las tablas nuevas necesitan su propio GRANT: el de 0001 solo alcanzó a las
-- tablas existentes en aquel momento. Sin esto, integrations_app no podría operar.
GRANT SELECT, INSERT, UPDATE, DELETE ON policies         TO integrations_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON policy_decisions TO integrations_app;
