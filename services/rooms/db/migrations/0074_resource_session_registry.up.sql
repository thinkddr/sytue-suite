-- SPDX-License-Identifier: Apache-2.0
-- Public product/device trust and consumed-consent metadata only. SDK access
-- tokens, SSSS secrets, private recipients and application content never enter.
ALTER TABLE matrix_native_consents ADD COLUMN purpose text NOT NULL DEFAULT ''
 CHECK(purpose='' OR (purpose='resources:'||product AND identity_version=1 AND native_consumer=''));
ALTER TABLE matrix_native_consents ADD COLUMN resource_approved_at timestamptz;
ALTER TABLE matrix_native_consents ADD COLUMN resource_client_id text NOT NULL DEFAULT '' CHECK((purpose='' AND resource_client_id='') OR (purpose<>'' AND resource_client_id IN('sytue-desktop','sytue-mobile')));
ALTER TABLE matrix_native_consents ADD CONSTRAINT matrix_native_resource_approval CHECK(resource_approved_at IS NULL OR purpose<>'');
CREATE POLICY matrix_native_consents_resource_contract ON matrix_native_consents AS RESTRICTIVE
 USING(purpose='' OR current_setting('app.matrix_resource_session_contract',true)='1')
 WITH CHECK(purpose='' OR current_setting('app.matrix_resource_session_contract',true)='1');

CREATE TABLE matrix_resource_consents (
 id uuid PRIMARY KEY,
 tenant text NOT NULL,subject text NOT NULL,product text NOT NULL,
 namespace text NOT NULL CHECK(namespace='resources:'||product),
 account_version bigint NOT NULL CHECK(account_version>=0),tenant_version bigint NOT NULL CHECK(tenant_version>=0),
 source text NOT NULL CHECK(source IN('native','web')),
 client_id text NOT NULL,
 grant_hash text NOT NULL UNIQUE CHECK(grant_hash~'^[0-9a-f]{64}$'),
 source_hash text NOT NULL UNIQUE CHECK(source_hash~'^[0-9a-f]{64}$'),
 approved_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL,
 consumed_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(expires_at>approved_at),
 CHECK((source='native' AND client_id IN('sytue-desktop','sytue-mobile')) OR
       (source='web' AND client_id NOT IN('sytue-desktop','sytue-mobile')))
);
CREATE TABLE matrix_resource_authorities (
 tenant text NOT NULL,subject text NOT NULL,product text NOT NULL,
 namespace text NOT NULL CHECK(namespace='resources:'||product),
 account_version bigint NOT NULL CHECK(account_version>=0),tenant_version bigint NOT NULL CHECK(tenant_version>=0),
 matrix_user_id text NOT NULL CHECK(matrix_user_id~'^@sytue_[0-9a-f-]{36}:'),
 master_key text NOT NULL CHECK(master_key~'^[A-Za-z0-9+/]{43}$'),
 recipient text NOT NULL,
 product_key_id text NOT NULL,
 generation bigint NOT NULL CHECK(generation>0),
 status text NOT NULL CHECK(status IN('active','revoked')),
 enrollment_sha256 text NOT NULL CHECK(enrollment_sha256~'^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant,subject,product)
);
CREATE TABLE matrix_resource_session_challenges (
 id uuid PRIMARY KEY,
 consent_id uuid NOT NULL UNIQUE REFERENCES matrix_resource_consents(id),
 tenant text NOT NULL,subject text NOT NULL,product text NOT NULL,
 namespace text NOT NULL CHECK(namespace='resources:'||product),
 account_version bigint NOT NULL CHECK(account_version>=0),tenant_version bigint NOT NULL CHECK(tenant_version>=0),
 matrix_user_id text NOT NULL,device_id text NOT NULL,
 master_key text NOT NULL,
 root_generation bigint NOT NULL CHECK(root_generation>0),
 generation bigint NOT NULL CHECK(generation>0),
 challenge bytea NOT NULL,
 challenge_sha256 text NOT NULL CHECK(challenge_sha256=encode(digest(challenge,'sha256'),'hex')),
 signed_recipient bytea NOT NULL,
 device_evidence bytea NOT NULL,
 expires_at timestamptz NOT NULL,
 consumed_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE matrix_resource_devices (
 tenant text NOT NULL,subject text NOT NULL,product text NOT NULL,
 namespace text NOT NULL CHECK(namespace='resources:'||product),
 account_version bigint NOT NULL CHECK(account_version>=0),tenant_version bigint NOT NULL CHECK(tenant_version>=0),
 matrix_user_id text NOT NULL,device_id text NOT NULL,
 master_key text NOT NULL,
 recipient text NOT NULL,product_key_id text NOT NULL,
 root_generation bigint NOT NULL CHECK(root_generation>0),
 generation bigint NOT NULL CHECK(generation>0),
 status text NOT NULL CHECK(status IN('active','revoked')),
 challenge_id uuid NOT NULL UNIQUE REFERENCES matrix_resource_session_challenges(id),
 binding bytea NOT NULL,
 enrollment bytea NOT NULL,
 enrollment_sha256 text NOT NULL CHECK(enrollment_sha256=encode(digest(enrollment,'sha256'),'hex')),
 signed_recipient bytea NOT NULL,
 device_evidence bytea NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant,subject,product,device_id),
 -- A shared native audience or global cross-signing root does not permit an
 -- existing Mail SDK device to become a Docs device under a caller's label.
 UNIQUE(matrix_user_id,device_id),
 UNIQUE(tenant,subject,product,product_key_id),
 FOREIGN KEY(tenant,subject,product) REFERENCES matrix_resource_authorities(tenant,subject,product)
);
CREATE TABLE matrix_resource_routes (
 room_id text PRIMARY KEY,
 tenant text NOT NULL,product text NOT NULL,kind text NOT NULL,resource_id text NOT NULL,
 service_id text NOT NULL,
 registration bytea NOT NULL,
 registration_sha256 text NOT NULL CHECK(registration_sha256=encode(digest(registration,'sha256'),'hex')),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant,product,kind,resource_id)
);

DO $$ DECLARE relation_name text; BEGIN
 FOREACH relation_name IN ARRAY ARRAY['matrix_resource_consents','matrix_resource_authorities','matrix_resource_session_challenges','matrix_resource_devices'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',relation_name);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',relation_name);
  EXECUTE format('CREATE POLICY resource_session_owner ON %I USING(tenant=current_setting(''app.tenant'',true) AND subject=current_setting(''app.resource_session_subject'',true) AND product=current_setting(''app.resource_session_product'',true)) WITH CHECK(tenant=current_setting(''app.tenant'',true) AND subject=current_setting(''app.resource_session_subject'',true) AND product=current_setting(''app.resource_session_product'',true))',relation_name);
 END LOOP;
END $$;
ALTER TABLE matrix_resource_routes ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_resource_routes FORCE ROW LEVEL SECURITY;
CREATE POLICY resource_route_product ON matrix_resource_routes
 USING(tenant=current_setting('app.tenant',true) AND product=current_setting('app.resource_session_product',true))
 WITH CHECK(tenant=current_setting('app.tenant',true) AND product=current_setting('app.resource_session_product',true));

-- A purpose is fixed before opening the browser, and approval cannot move to a
-- different tenant/product. Legacy binaries do not set the resource contract.
CREATE FUNCTION matrix_resource_native_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' AND (OLD.purpose,OLD.tenant,OLD.subject,OLD.product,OLD.challenge,OLD.native_consumer,OLD.identity_version,OLD.state_hash,OLD.resource_client_id)
 IS DISTINCT FROM (NEW.purpose,NEW.tenant,NEW.subject,NEW.product,NEW.challenge,NEW.native_consumer,NEW.identity_version,NEW.state_hash,NEW.resource_client_id) THEN
  RAISE EXCEPTION 'native purpose is immutable' USING ERRCODE='55000';
 END IF;
 IF NEW.purpose<>'' AND current_setting('app.matrix_resource_session_contract',true) IS DISTINCT FROM '1' THEN
  RAISE EXCEPTION 'resource native contract required' USING ERRCODE='42501';
 END IF;
 IF TG_OP='UPDATE' AND OLD.resource_approved_at IS NOT NULL AND NEW.resource_approved_at IS DISTINCT FROM OLD.resource_approved_at THEN
  RAISE EXCEPTION 'resource native consent is immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER matrix_resource_native_guard BEFORE INSERT OR UPDATE ON matrix_native_consents FOR EACH ROW EXECUTE FUNCTION matrix_resource_native_guard();

CREATE FUNCTION matrix_resource_public_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'public resource trust must retain revocation evidence' USING ERRCODE='55000'; END IF;
 IF current_setting('app.tenant',true) IS DISTINCT FROM NEW.tenant OR current_setting('app.resource_session_product',true) IS DISTINCT FROM NEW.product

 OR coalesce(current_setting('app.merge',true),'') NOT IN('','false','off','0') THEN
  RAISE EXCEPTION 'resource trust scope mismatch' USING ERRCODE='42501';
 END IF;
 IF TG_TABLE_NAME<>'matrix_resource_routes' THEN
  IF current_setting('app.resource_session_subject',true) IS DISTINCT FROM NEW.subject THEN RAISE EXCEPTION 'resource trust subject mismatch' USING ERRCODE='42501'; END IF;
 END IF;
 IF TG_TABLE_NAME IN('matrix_resource_authorities','matrix_resource_devices') THEN
  IF current_setting('app.resource_session_confirm',true) IS DISTINCT FROM '1' AND TG_OP='INSERT' THEN
   RAISE EXCEPTION 'confirmed SDK product enrollment required' USING ERRCODE='42501';
  END IF;
  IF TG_OP='UPDATE' THEN
   IF (to_jsonb(OLD)-'status'-'updated_at') IS DISTINCT FROM (to_jsonb(NEW)-'status'-'updated_at') OR OLD.status<>'active' OR NEW.status<>'revoked' OR current_setting('app.resource_session_revoke',true) IS DISTINCT FROM '1' THEN
    RAISE EXCEPTION 'resource trust update requires a drained revocation' USING ERRCODE='55000';
   END IF;
  END IF;
 ELSIF TG_OP='UPDATE' THEN
  IF TG_TABLE_NAME='matrix_resource_routes' THEN RAISE EXCEPTION 'resource route is immutable' USING ERRCODE='55000'; END IF;
  IF (to_jsonb(OLD)-'consumed_at') IS DISTINCT FROM (to_jsonb(NEW)-'consumed_at') OR OLD.consumed_at IS NOT NULL OR NEW.consumed_at IS NULL THEN
   RAISE EXCEPTION 'resource consent evidence is immutable' USING ERRCODE='55000';
  END IF;
 END IF;
 RETURN NEW;
END $$;
DO $$ DECLARE relation_name text; BEGIN
 FOREACH relation_name IN ARRAY ARRAY['matrix_resource_consents','matrix_resource_authorities','matrix_resource_session_challenges','matrix_resource_devices','matrix_resource_routes'] LOOP
  EXECUTE format('CREATE TRIGGER resource_public_guard BEFORE INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION matrix_resource_public_guard()',relation_name);
 END LOOP;
END $$;

-- Older lifecycle writers cannot see resource-purpose rows through RLS. Retire
-- exactly these opaque handoffs atomically with durable identity/tenant denial.
CREATE FUNCTION matrix_native_resources_retire() RETURNS trigger LANGUAGE plpgsql SECURITY INVOKER SET search_path=pg_catalog,public AS $$
DECLARE previous_contract text;
BEGIN
 IF NEW.status='active' THEN RETURN NEW; END IF;
 previous_contract:=current_setting('app.matrix_resource_session_contract',true);
 PERFORM set_config('app.matrix_resource_session_contract','1',true);
 IF TG_TABLE_NAME='matrix_identity_access' THEN
  DELETE FROM public.matrix_native_consents WHERE subject=NEW.subject AND purpose<>'';
 ELSE
  DELETE FROM public.matrix_native_consents WHERE tenant=NEW.tenant AND subject=NEW.subject AND purpose<>'';
 END IF;
 PERFORM set_config('app.matrix_resource_session_contract',COALESCE(previous_contract,''),true);
 RETURN NEW;
END $$;
CREATE TRIGGER matrix_native_resources_retire_identity AFTER INSERT OR UPDATE OF status ON matrix_identity_access FOR EACH ROW EXECUTE FUNCTION matrix_native_resources_retire();
CREATE TRIGGER matrix_native_resources_retire_tenant AFTER INSERT OR UPDATE OF status ON matrix_tenant_access FOR EACH ROW EXECUTE FUNCTION matrix_native_resources_retire();
SELECT sytue_scope_merge_register('matrix_resource_consents'::regclass);
SELECT sytue_scope_merge_register('matrix_resource_authorities'::regclass);
SELECT sytue_scope_merge_register('matrix_resource_session_challenges'::regclass);
SELECT sytue_scope_merge_register('matrix_resource_devices'::regclass);
SELECT sytue_scope_merge_register('matrix_resource_routes'::regclass);
SELECT sytue_scope_admission_register('matrix_resource_consents'::regclass,false);
SELECT sytue_scope_admission_register('matrix_resource_authorities'::regclass,false);
SELECT sytue_scope_admission_register('matrix_resource_session_challenges'::regclass,false);
SELECT sytue_scope_admission_register('matrix_resource_devices'::regclass,false);
SELECT sytue_scope_admission_register('matrix_resource_routes'::regclass,false);
