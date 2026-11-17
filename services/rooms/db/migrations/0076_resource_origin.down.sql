-- SPDX-License-Identifier: Apache-2.0
DO $$BEGIN
 IF EXISTS(SELECT 1 FROM matrix_resource_routes WHERE registration_kind='bootstrap-v1')THEN
  RAISE EXCEPTION 'registered resource route evidence must be retained' USING ERRCODE='55000';
 END IF;
END $$;
DROP POLICY resource_route_origin_lookup ON matrix_resource_routes;
DROP TRIGGER matrix_resource_route_bootstrap_guard ON matrix_resource_routes;
DROP FUNCTION matrix_resource_route_bootstrap_guard();
ALTER TABLE matrix_resource_routes DROP CONSTRAINT matrix_resource_route_bootstrap_fields,
 DROP COLUMN registration_kind,DROP COLUMN plan_id,DROP COLUMN plan_sha256,
 DROP COLUMN actor_subject,DROP COLUMN actor_user,DROP COLUMN actor_device,DROP COLUMN actor_root,
 DROP COLUMN actor_generation,DROP COLUMN actor_binding_sha256,DROP COLUMN source_epoch,DROP COLUMN source_seq,DROP COLUMN acl_version;
