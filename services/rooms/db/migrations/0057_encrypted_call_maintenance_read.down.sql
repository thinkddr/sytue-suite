-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN
 PERFORM set_config('app.maintenance','on',true);
 IF EXISTS(SELECT 1 FROM matrix_call_epochs) THEN RAISE EXCEPTION 'Encrypted call revocation still requires maintenance enumeration';END IF;
END $$;
DROP POLICY rooms_encrypted_call_maintenance_read ON rooms;
