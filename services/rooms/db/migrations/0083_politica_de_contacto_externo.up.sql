-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- LA ORGANIZACIÓN DECIDE EL CONTACTO CON GENTE DE FUERA EN ROOMS (ADR 0040; como la de
-- las peticiones de firma de CertiPDF, ADR 0059). Abierta (la de siempre, y la de una
-- organización sin fila), cerrada, o solo con las organizaciones de `confianza` (sus
-- slugs, hasta 100). Vale en los dos sentidos y para todo lo de una conversación de
-- fuera —canales, DM, grupos, sus llamadas y la reunión de un evento—:
--
--   * quien es de esta organización no abre conversaciones con nadie que no admita, ni
--     entra desde sus sesiones en las de otra que no admita;
--   * nadie de una organización que no admite entra en las conversaciones de esta.
--
-- La guarda quien administra (owner o admin) desde Sytue Admin.
CREATE TABLE politica_contacto_externo (
    tenant          text        PRIMARY KEY,
    modo            text        NOT NULL DEFAULT 'abierta',
    confianza       text[]      NOT NULL DEFAULT '{}',
    actualizada     timestamptz NOT NULL DEFAULT now(),
    actualizada_por text        NOT NULL DEFAULT '',
    CONSTRAINT politica_contacto_externo_modo_chk CHECK (modo IN ('abierta', 'cerrada', 'confianza')),
    CONSTRAINT politica_contacto_externo_confianza_chk CHECK (cardinality(confianza) <= 100)
);
ALTER TABLE politica_contacto_externo ENABLE ROW LEVEL SECURITY;
ALTER TABLE politica_contacto_externo FORCE ROW LEVEL SECURITY;
CREATE POLICY politica_contacto_externo_tenant_isolation ON politica_contacto_externo
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY politica_contacto_externo_platform_merge ON politica_contacto_externo
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

-- En una fusión (y un renombrado, que usa el mismo motor) la política del origen se
-- descarta si el destino tiene la suya, y quien confiaba en el origen pasa a confiar
-- en el destino, sin repetidos ni confiar en sí misma: como la de CertiPDF. Lo demás
-- es la preparación de siempre (mig 0053).
CREATE OR REPLACE FUNCTION platform_merge_prepare(source_tenant text,target_tenant text) RETURNS text
LANGUAGE plpgsql AS $$
DECLARE merged bigint; discarded bigint; politicas bigint;
BEGIN
    IF current_setting('app.merge',true) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION 'Organization merge context required';
    END IF;
    IF source_tenant=target_tenant THEN RETURN NULL; END IF;
    IF EXISTS(SELECT 1 FROM matrix_tenant_access s JOIN matrix_tenant_access d USING(subject)
        WHERE s.tenant=source_tenant AND d.tenant=target_tenant AND s.version=d.version AND s.status<>d.status) THEN
        RAISE EXCEPTION 'Conflicting Matrix lifecycle commands share a version';
    END IF;
    UPDATE matrix_tenant_access d SET status=s.status,version=s.version,applied=s.applied
        FROM matrix_tenant_access s WHERE s.tenant=source_tenant AND d.tenant=target_tenant
        AND s.subject=d.subject AND s.version>d.version;
    DELETE FROM matrix_tenant_access s USING matrix_tenant_access d
        WHERE s.tenant=source_tenant AND d.tenant=target_tenant AND s.subject=d.subject;
    GET DIAGNOSTICS merged=ROW_COUNT;
    DELETE FROM matrix_native_consents WHERE tenant=source_tenant;
    GET DIAGNOSTICS discarded=ROW_COUNT;
    DELETE FROM politica_contacto_externo s
        WHERE s.tenant=source_tenant
          AND EXISTS (SELECT 1 FROM politica_contacto_externo d WHERE d.tenant=target_tenant);
    GET DIAGNOSTICS politicas=ROW_COUNT;
    UPDATE politica_contacto_externo p SET confianza = ARRAY(
            SELECT DISTINCT CASE WHEN o = source_tenant THEN target_tenant ELSE o END
            FROM unnest(p.confianza) AS o
            WHERE (CASE WHEN o = source_tenant THEN target_tenant ELSE o END)
                  <> (CASE WHEN p.tenant = source_tenant THEN target_tenant ELSE p.tenant END))
        WHERE source_tenant = ANY (p.confianza);
    IF merged>0 OR discarded>0 OR politicas>0 THEN
        RETURN 'Matrix lifecycle versions retained; expired tenant-bound native sign-ins discarded; encrypted draft rooms preserved; the target external contact policy prevails';
    END IF;
    RETURN NULL;
END $$;
