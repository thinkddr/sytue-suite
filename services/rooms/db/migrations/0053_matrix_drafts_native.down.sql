-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM matrix_draft_rooms) THEN
        RAISE EXCEPTION 'Encrypted draft room references must be preserved before rollback';
    END IF;
END $$;
DROP TABLE matrix_native_consents;
DROP TABLE matrix_draft_rooms;

CREATE OR REPLACE FUNCTION platform_merge_prepare(source_tenant text,target_tenant text) RETURNS text
LANGUAGE plpgsql AS $$
DECLARE merged bigint;
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
    IF merged>0 THEN RETURN 'Matrix lifecycle access merged by highest ID version'; END IF;
    RETURN NULL;
END $$;
