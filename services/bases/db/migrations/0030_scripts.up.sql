-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Scripts de usuario de Sytue Bases (bases-2, automatización; ADR 0069).
--
-- El guion cuelga de UNA base; sus versiones son INMUTABLES (el código y el manifiesto no
-- se actualizan jamás: editar crea otra versión, y un trigger lo impide también por SQL);
-- la propietaria de la base aprueba la versión de un script compartido; y cada ejecución
-- deja una línea, de la que salen también las cuotas. El servidor no ejecuta el código.
--
-- Las mismas reglas que el resto de Bases (ver 0001 y 0004): RLS forzada con las dos
-- políticas y claves foráneas COMPUESTAS (tenant, id) y diferidas —un script no puede
-- colgar de una base de otra organización aunque el código se equivocara, y la fusión de
-- organizaciones reescribe `tenant` tabla a tabla—.
CREATE TABLE base_scripts (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant     text NOT NULL,
    base_id    uuid NOT NULL,
    name       text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    -- 'private': solo su autora. 'shared': lo ejecutan quienes acceden a la base, pero
    -- solo una versión aprobada.
    scope      text NOT NULL DEFAULT 'private' CHECK (scope IN ('private', 'shared')),
    author_sub text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant, id),
    FOREIGN KEY (tenant, base_id) REFERENCES bases (tenant, id)
        ON DELETE CASCADE ON UPDATE CASCADE DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX base_scripts_por_base ON base_scripts (tenant, base_id, created_at DESC);

CREATE TABLE base_script_versions (
    tenant      text NOT NULL,
    script_id   uuid NOT NULL,
    version     integer NOT NULL CHECK (version >= 1),
    code        text NOT NULL CHECK (length(code) BETWEEN 1 AND 65536),
    -- SHA-256 hex del código: viaja con la aprobación, de modo que lo aprobado es ESTE código.
    code_hash   text NOT NULL CHECK (code_hash ~ '^[0-9a-f]{64}$'),
    manifest    jsonb NOT NULL,
    -- draft -> pending (la autora la propone) -> approved | rejected (la propietaria).
    status      text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'pending', 'approved', 'rejected')),
    created_by  text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    reviewed_by text,
    reviewed_at timestamptz,
    PRIMARY KEY (script_id, version),
    FOREIGN KEY (tenant, script_id) REFERENCES base_scripts (tenant, id)
        ON DELETE CASCADE ON UPDATE CASCADE DEFERRABLE INITIALLY DEFERRED
);

-- Inmutabilidad: el código, su hash y el manifiesto no cambian nunca, ni por SQL directo.
-- (`tenant` sí puede cambiar: lo reescribe la fusión de organizaciones.)
CREATE FUNCTION base_script_versions_inmutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.code IS DISTINCT FROM OLD.code
       OR NEW.code_hash IS DISTINCT FROM OLD.code_hash
       OR NEW.manifest IS DISTINCT FROM OLD.manifest
       OR NEW.version IS DISTINCT FROM OLD.version
       OR NEW.script_id IS DISTINCT FROM OLD.script_id THEN
        RAISE EXCEPTION 'base_script_versions: el código y el manifiesto de una versión son inmutables';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER base_script_versions_inmutable BEFORE UPDATE ON base_script_versions
    FOR EACH ROW EXECUTE FUNCTION base_script_versions_inmutable();

-- El registro de ejecuciones es auditoría: SIN claves foráneas, sobrevive al borrado del
-- script y de la base (como base_audit).
CREATE TABLE base_script_runs (
    id          bigserial PRIMARY KEY,
    tenant      text NOT NULL,
    base_id     uuid NOT NULL,
    script_id   uuid NOT NULL,
    version     integer NOT NULL,
    code_hash   text NOT NULL,
    sub         text NOT NULL,
    -- Tiempo reservado (el máximo del manifiesto); cuenta contra la cuota mientras 'running'.
    max_ms      integer NOT NULL CHECK (max_ms > 0),
    outcome     text NOT NULL DEFAULT 'running' CHECK (outcome IN ('running', 'ok', 'error', 'denied', 'timeout', 'limit')),
    -- Código de error corto (nunca el contenido de lo leído ni de lo escrito).
    error_code  text NOT NULL DEFAULT '' CHECK (error_code ~ '^[a-z0-9_.-]{0,64}$'),
    duration_ms integer CHECK (duration_ms >= 0),
    ops         integer NOT NULL DEFAULT 0 CHECK (ops >= 0),
    started_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX base_script_runs_cuota ON base_script_runs (tenant, started_at DESC);
CREATE INDEX base_script_runs_por_base ON base_script_runs (tenant, base_id, id DESC);

ALTER TABLE base_scripts ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_scripts FORCE ROW LEVEL SECURITY;
CREATE POLICY base_scripts_tenant_isolation ON base_scripts
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_scripts_platform_merge ON base_scripts
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

ALTER TABLE base_script_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_script_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY base_script_versions_tenant_isolation ON base_script_versions
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_script_versions_platform_merge ON base_script_versions
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

ALTER TABLE base_script_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_script_runs FORCE ROW LEVEL SECURITY;
CREATE POLICY base_script_runs_tenant_isolation ON base_script_runs
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_script_runs_platform_merge ON base_script_runs
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
