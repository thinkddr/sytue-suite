-- SPDX-License-Identifier: Apache-2.0
-- Trust anchors and revocation evidence are deliberately not removed by an
-- ordinary rollback. Their retirement needs a separate verified migration.
DO $$ BEGIN
 RAISE EXCEPTION 'matrix authority history prevents automatic rollback' USING ERRCODE='55000';
END $$;
