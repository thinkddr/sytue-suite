-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- AUTOMATIZACIÓN SEGURA (ROADMAP-OFFICE automatizacion-1, pieza A).
--
-- Scripts de usuario de Docus, Calcus y Diapos: el guion, sus versiones INMUTABLES (el
-- código y el manifiesto de permisos no se actualizan jamás; editar crea otra versión),
-- la aprobación por un administrador cuando se comparten con la organización y el
-- registro de cada ejecución, del que salen también las cuotas. Tablas nuevas.
CREATE TABLE doc_scripts (
    tenant       text        NOT NULL,
    id           uuid        NOT NULL DEFAULT gen_random_uuid(),
    name         text        NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    kind         text        NOT NULL CHECK (kind IN ('document', 'spreadsheet', 'presentation')),
    -- 'private': solo su autor. 'org': lo ejecutan otros, pero solo versiones aprobadas.
    scope        text        NOT NULL DEFAULT 'private' CHECK (scope IN ('private', 'org')),
    -- Opcional: el script solo se ejecuta sobre ese documento.
    document_id  uuid,
    author_sub   text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz,
    PRIMARY KEY (tenant, id)
);

CREATE TABLE doc_script_versions (
    tenant       text        NOT NULL,
    script_id    uuid        NOT NULL,
    version      integer     NOT NULL CHECK (version >= 1),
    code         text        NOT NULL CHECK (length(code) <= 65536),
    -- SHA-256 hex del código: viaja con la aprobación, de modo que lo aprobado es ESTE código.
    code_hash    text        NOT NULL CHECK (code_hash ~ '^[0-9a-f]{64}$'),
    manifest     jsonb       NOT NULL,
    -- draft -> pending (el autor la propone) -> approved | rejected (un administrador).
    status       text        NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'pending', 'approved', 'rejected')),
    created_by   text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    reviewed_by  text,
    reviewed_at  timestamptz,
    PRIMARY KEY (tenant, script_id, version),
    FOREIGN KEY (tenant, script_id) REFERENCES doc_scripts (tenant, id) ON DELETE CASCADE
);

-- Inmutabilidad: el código, su hash y el manifiesto no cambian nunca, ni por SQL directo.
CREATE FUNCTION doc_script_versions_inmutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.code IS DISTINCT FROM OLD.code
       OR NEW.code_hash IS DISTINCT FROM OLD.code_hash
       OR NEW.manifest IS DISTINCT FROM OLD.manifest
       OR NEW.version IS DISTINCT FROM OLD.version
       OR NEW.script_id IS DISTINCT FROM OLD.script_id THEN
        RAISE EXCEPTION 'doc_script_versions: el código y el manifiesto de una versión son inmutables';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER doc_script_versions_inmutable BEFORE UPDATE ON doc_script_versions
    FOR EACH ROW EXECUTE FUNCTION doc_script_versions_inmutable();

CREATE TABLE doc_script_runs (
    tenant       text        NOT NULL,
    id           bigserial   NOT NULL,
    script_id    uuid        NOT NULL,
    version      integer     NOT NULL,
    code_hash    text        NOT NULL,
    document_id  uuid        NOT NULL,
    sub          text        NOT NULL,
    -- Tiempo reservado (el máximo del manifiesto); cuenta contra la cuota mientras 'running'.
    max_ms       integer     NOT NULL CHECK (max_ms > 0),
    outcome      text        NOT NULL DEFAULT 'running' CHECK (outcome IN ('running', 'ok', 'error', 'denied', 'timeout', 'limit')),
    -- Código de error corto (nunca el contenido de lo leído ni de lo escrito).
    error_code   text        NOT NULL DEFAULT '' CHECK (length(error_code) <= 64),
    duration_ms  integer     CHECK (duration_ms >= 0),
    ops          integer     NOT NULL DEFAULT 0 CHECK (ops >= 0),
    started_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, id)
);
CREATE INDEX doc_script_runs_sub_idx ON doc_script_runs (tenant, sub, started_at DESC);
CREATE INDEX doc_script_runs_doc_idx ON doc_script_runs (tenant, document_id, id DESC);

ALTER TABLE doc_scripts ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_scripts FORCE ROW LEVEL SECURITY;
CREATE POLICY doc_scripts_tenant_isolation ON doc_scripts
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_scripts_platform_merge ON doc_scripts
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
GRANT SELECT, INSERT, UPDATE, DELETE ON doc_scripts TO docs_app;

ALTER TABLE doc_script_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_script_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY doc_script_versions_tenant_isolation ON doc_script_versions
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_script_versions_platform_merge ON doc_script_versions
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
GRANT SELECT, INSERT, UPDATE, DELETE ON doc_script_versions TO docs_app;

ALTER TABLE doc_script_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_script_runs FORCE ROW LEVEL SECURITY;
CREATE POLICY doc_script_runs_tenant_isolation ON doc_script_runs
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_script_runs_platform_merge ON doc_script_runs
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
GRANT SELECT, INSERT, UPDATE, DELETE ON doc_script_runs TO docs_app;

GRANT USAGE ON SEQUENCE doc_script_runs_id_seq TO docs_app;
