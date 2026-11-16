-- SPDX-License-Identifier: Apache-2.0
-- Only public pre-epoch routing metadata. The 0069 publication ledger remains
-- the sole origin ledger; recognizing a route does not authorize publication.
ALTER TABLE matrix_resource_routes
 ADD COLUMN registration_kind text NOT NULL DEFAULT 'legacy' CHECK(registration_kind IN('legacy','bootstrap-v1')),
 ADD COLUMN plan_id uuid,
 ADD COLUMN plan_sha256 text,
 ADD COLUMN actor_subject text,
 ADD COLUMN actor_user text,
 ADD COLUMN actor_device text,
 ADD COLUMN actor_root text,
 ADD COLUMN actor_generation bigint,
 ADD COLUMN actor_binding_sha256 text,
 ADD COLUMN source_epoch uuid,
 ADD COLUMN source_seq bigint,
 ADD COLUMN acl_version bigint,
 ADD CONSTRAINT matrix_resource_route_bootstrap_fields CHECK(
  registration_kind='legacy' OR(
   plan_id IS NOT NULL AND plan_sha256 IS NOT NULL AND plan_sha256~'^[a-f0-9]{64}$'
   AND actor_subject IS NOT NULL AND actor_subject<>'' AND actor_user IS NOT NULL AND left(actor_user,1)='@'
   AND actor_device IS NOT NULL AND actor_device<>'' AND actor_root IS NOT NULL AND actor_root<>''
   AND actor_generation IS NOT NULL AND actor_generation>0
   AND actor_binding_sha256 IS NOT NULL AND actor_binding_sha256~'^[a-f0-9]{64}$'
   AND source_epoch IS NOT NULL AND source_seq IS NOT NULL AND source_seq>=0 AND acl_version IS NOT NULL AND acl_version>=0
  )
 );

CREATE FUNCTION matrix_resource_route_bootstrap_guard()RETURNS trigger
 LANGUAGE plpgsql SECURITY INVOKER SET search_path=pg_catalog,public AS $$
DECLARE p jsonb;
BEGIN
 IF current_setting('app.matrix_resource_route_contract',true) IS DISTINCT FROM '1'
 OR NEW.registration_kind<>'bootstrap-v1' THEN
  RAISE EXCEPTION 'verified resource route contract required' USING ERRCODE='42501';
 END IF;
 p:=convert_from(NEW.registration,'UTF8')::jsonb;
 IF jsonb_typeof(p) IS DISTINCT FROM 'object'
 OR p->>'v' IS DISTINCT FROM '1'
 OR p->>'domain' IS DISTINCT FROM 'org.sytue.resource.bootstrap.plan.v1'
 OR p->>'id' IS DISTINCT FROM NEW.plan_id::text
 OR p->>'room_id' IS DISTINCT FROM NEW.room_id
 OR p#>>'{source,resource,tenant}' IS DISTINCT FROM NEW.tenant
 OR p#>>'{source,resource,product}' IS DISTINCT FROM NEW.product
 OR p#>>'{source,resource,kind}' IS DISTINCT FROM NEW.kind
 OR p#>>'{source,resource,id}' IS DISTINCT FROM NEW.resource_id
 OR p#>>'{source,owner_scope,tenant}' IS DISTINCT FROM NEW.tenant
 OR p#>>'{source,owner_scope,product}' IS DISTINCT FROM NEW.product
 OR p#>>'{source,owner_scope,subject}' IS DISTINCT FROM NEW.actor_subject
 OR p#>>'{source,source_epoch}' IS DISTINCT FROM NEW.source_epoch::text
 OR p#>>'{source,source_seq}' IS DISTINCT FROM NEW.source_seq::text
 OR p#>>'{source,acl_version}' IS DISTINCT FROM NEW.acl_version::text
 OR p#>>'{source,history}' IS DISTINCT FROM 'full'
 OR p->>'history' IS DISTINCT FROM 'full'
 OR p#>>'{creator_scope,tenant}' IS DISTINCT FROM NEW.tenant
 OR p#>>'{creator_scope,product}' IS DISTINCT FROM NEW.product
 OR p#>>'{creator_scope,subject}' IS DISTINCT FROM NEW.actor_subject
 OR p#>>'{creator,user_id}' IS DISTINCT FROM NEW.actor_user
 OR p#>>'{creator,device_id}' IS DISTINCT FROM NEW.actor_device
 OR p#>>'{creator,root_key}' IS DISTINCT FROM NEW.actor_root
 OR p->>'session_binding_sha256' IS DISTINCT FROM NEW.actor_binding_sha256
 OR NEW.service_id IS DISTINCT FROM (CASE WHEN NEW.product IN('docs','sheets','slides')THEN 'docs' ELSE NEW.product END)
 OR current_setting('app.resource_session_subject',true) IS DISTINCT FROM NEW.actor_subject
 OR (p-ARRAY['v','domain','id','source','room_id','epoch','recipient','membership_sha256','history','validation','creator_scope','creator','session_binding_sha256','signatures'])<>'{}'::jsonb
 OR NOT EXISTS(SELECT 1 FROM public.matrix_resource_devices d JOIN public.matrix_resource_authorities a USING(tenant,subject,product)
  WHERE d.tenant=NEW.tenant AND d.subject=NEW.actor_subject AND d.product=NEW.product
   AND d.device_id=NEW.actor_device AND d.matrix_user_id=NEW.actor_user AND d.master_key=NEW.actor_root
   AND d.generation=NEW.actor_generation AND d.status='active'
   AND a.tenant=NEW.tenant AND a.subject=NEW.actor_subject AND a.product=NEW.product
   AND a.status='active' AND a.matrix_user_id=d.matrix_user_id AND a.master_key=d.master_key AND a.generation=d.root_generation
 )THEN
  RAISE EXCEPTION 'resource route plan or enrollment mismatch' USING ERRCODE='42501';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER matrix_resource_route_bootstrap_guard BEFORE INSERT ON matrix_resource_routes
 FOR EACH ROW EXECUTE FUNCTION matrix_resource_route_bootstrap_guard();

-- The one pre-tenant lookup can read only its exact public room registration.
-- Normal reads still use tenant/product RLS; this policy never permits writes.
CREATE POLICY resource_route_origin_lookup ON matrix_resource_routes FOR SELECT
 USING(current_setting('app.maintenance',true)='on'
  AND room_id=current_setting('app.matrix_resource_route_room',true));
