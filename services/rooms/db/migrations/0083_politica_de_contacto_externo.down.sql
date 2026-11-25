-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira la política de contacto con gente de fuera y deja la preparación de la
-- fusión como en la 0053. Bajar abre el contacto en todas las organizaciones.

CREATE OR REPLACE FUNCTION platform_merge_prepare(source_tenant text,target_tenant text) RETURNS text
LANGUAGE plpgsql AS $$
DECLARE merged bigint; discarded bigint;
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
    IF merged>0 OR discarded>0 THEN
        RETURN 'Matrix lifecycle versions retained; expired tenant-bound native sign-ins discarded; encrypted draft rooms preserved';
    END IF;
    RETURN NULL;
END $$;

-- sytue:destructive-ok deshace 0083: la tabla solo la usa el binario nuevo.
DROP TABLE IF EXISTS politica_contacto_externo;
