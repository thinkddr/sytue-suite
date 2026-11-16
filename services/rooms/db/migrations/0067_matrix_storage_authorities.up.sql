-- SPDX-License-Identifier: Apache-2.0
-- Public trust anchors only. No recovery key, device private key or message
-- content is stored by this authority. Product identities remain isolated.
CREATE TABLE matrix_storage_authorities (
 tenant text NOT NULL,
 subject text NOT NULL,
 product text NOT NULL CHECK(product IN('hub','mail','drive','docs','sheets','slides','pdf','calendar','contacts','notes','photos','rooms','cast','erp','bases')),
 matrix_user_id text NOT NULL CHECK(octet_length(matrix_user_id) BETWEEN 3 AND 255),
 master_key text NOT NULL CHECK(master_key ~ '^[A-Za-z0-9+/]{43}$'),
 generation bigint NOT NULL CHECK(generation>0 AND generation<9007199254740992),
 status text NOT NULL CHECK(status IN('active','revoked')),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant,subject,product)
);
CREATE TABLE matrix_storage_authority_history (
 tenant text NOT NULL,subject text NOT NULL,product text NOT NULL,
 generation bigint NOT NULL CHECK(generation>0 AND generation<9007199254740992),
 master_key text NOT NULL CHECK(master_key ~ '^[A-Za-z0-9+/]{43}$'),
 status text NOT NULL CHECK(status IN('active','revoked')),
 reason text NOT NULL CHECK(reason IN('enroll','rotate','revoke','resume')),
 device_id text NOT NULL CHECK(octet_length(device_id) BETWEEN 1 AND 255),
 evidence jsonb NOT NULL CHECK(jsonb_typeof(evidence)='object' AND octet_length(evidence::text)<=65536),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant,subject,product,generation),
 FOREIGN KEY(tenant,subject,product) REFERENCES matrix_storage_authorities(tenant,subject,product)
);
ALTER TABLE matrix_storage_authorities ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_storage_authorities FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_storage_authorities_owner ON matrix_storage_authorities
 USING(tenant=current_setting('app.tenant',true) AND subject=current_setting('app.matrix_authority_owner',true) AND product=current_setting('app.matrix_authority_product',true))
 WITH CHECK(tenant=current_setting('app.tenant',true) AND subject=current_setting('app.matrix_authority_owner',true) AND product=current_setting('app.matrix_authority_product',true));
ALTER TABLE matrix_storage_authority_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_storage_authority_history FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_storage_authority_history_owner ON matrix_storage_authority_history
 USING(tenant=current_setting('app.tenant',true) AND subject=current_setting('app.matrix_authority_owner',true) AND product=current_setting('app.matrix_authority_product',true))
 WITH CHECK(tenant=current_setting('app.tenant',true) AND subject=current_setting('app.matrix_authority_owner',true) AND product=current_setting('app.matrix_authority_product',true));

CREATE FUNCTION matrix_storage_authority_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'matrix authority revocation must be retained' USING ERRCODE='55000'; END IF;
 IF current_setting('app.tenant',true) IS DISTINCT FROM NEW.tenant
 OR current_setting('app.matrix_authority_owner',true) IS DISTINCT FROM NEW.subject
 OR current_setting('app.matrix_authority_product',true) IS DISTINCT FROM NEW.product
 OR coalesce(current_setting('app.merge',true),'') NOT IN('','false','off','0') THEN
  RAISE EXCEPTION 'matrix authority scope mismatch' USING ERRCODE='42501';
 END IF;
 IF TG_OP='INSERT' AND (NEW.generation<>1 OR NEW.status<>'active' OR current_setting('app.matrix_authority_change',true) IS DISTINCT FROM 'enroll') THEN
  RAISE EXCEPTION 'matrix authority enrollment proof required' USING ERRCODE='42501';
 END IF;
 IF TG_OP='UPDATE' THEN
  IF (OLD.tenant,OLD.subject,OLD.product,OLD.matrix_user_id,OLD.created_at) IS DISTINCT FROM (NEW.tenant,NEW.subject,NEW.product,NEW.matrix_user_id,NEW.created_at)
  OR NEW.generation<>OLD.generation+1 THEN
   RAISE EXCEPTION 'matrix authority generation is immutable or monotonic' USING ERRCODE='55000';
  END IF;
  IF NOT (
   (current_setting('app.matrix_authority_change',true)='revoke' AND OLD.status='active' AND NEW.status='revoked' AND NEW.master_key=OLD.master_key)
   OR (current_setting('app.matrix_authority_change',true)='resume' AND OLD.status='revoked' AND NEW.status='active' AND NEW.master_key=OLD.master_key)
   OR (current_setting('app.matrix_authority_change',true)='rotate' AND OLD.status='active' AND NEW.status='active' AND NEW.master_key<>OLD.master_key)
  ) THEN RAISE EXCEPTION 'matrix authority transition proof required' USING ERRCODE='42501'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER matrix_storage_authority_guard BEFORE INSERT OR UPDATE OR DELETE ON matrix_storage_authorities FOR EACH ROW EXECUTE FUNCTION matrix_storage_authority_guard();

CREATE FUNCTION matrix_storage_authority_history_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'matrix authority history is immutable' USING ERRCODE='55000'; END IF;
 IF current_setting('app.tenant',true) IS DISTINCT FROM NEW.tenant
 OR current_setting('app.matrix_authority_owner',true) IS DISTINCT FROM NEW.subject
 OR current_setting('app.matrix_authority_product',true) IS DISTINCT FROM NEW.product
 OR current_setting('app.matrix_authority_change',true) IS DISTINCT FROM NEW.reason
 OR coalesce(current_setting('app.merge',true),'') NOT IN('','false','off','0')
 OR NOT EXISTS(SELECT 1 FROM matrix_storage_authorities a WHERE a.tenant=NEW.tenant AND a.subject=NEW.subject AND a.product=NEW.product AND a.generation=NEW.generation AND a.master_key=NEW.master_key AND a.status=NEW.status) THEN
  RAISE EXCEPTION 'matrix authority history scope or generation mismatch' USING ERRCODE='42501';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER matrix_storage_authority_history_guard BEFORE INSERT OR UPDATE OR DELETE ON matrix_storage_authority_history FOR EACH ROW EXECUTE FUNCTION matrix_storage_authority_history_guard();

-- Consent challenges belong to Rooms; delivery claim/ACK nonces remain in the
-- owning Mail/Calendar database so those operations can consume them atomically.
CREATE TABLE matrix_storage_root_consents (
 tenant text NOT NULL,subject text NOT NULL,product text NOT NULL,
 nonce uuid NOT NULL,
 action jsonb NOT NULL CHECK(jsonb_typeof(action)='object' AND octet_length(action::text)<=16384),
 request jsonb NOT NULL CHECK(jsonb_typeof(request)='object' AND octet_length(request::text)<=16384),
 created_at timestamptz NOT NULL DEFAULT now(),expires_at timestamptz NOT NULL,
 consumed_at timestamptz,
 PRIMARY KEY(tenant,subject,product,nonce),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '6 minutes')
);
CREATE INDEX matrix_storage_root_consents_pending ON matrix_storage_root_consents(tenant,subject,product,expires_at) WHERE consumed_at IS NULL;
ALTER TABLE matrix_storage_root_consents ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_storage_root_consents FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_storage_root_consents_owner ON matrix_storage_root_consents
 USING(tenant=current_setting('app.tenant',true) AND subject=current_setting('app.matrix_authority_owner',true) AND product=current_setting('app.matrix_authority_product',true))
 WITH CHECK(tenant=current_setting('app.tenant',true) AND subject=current_setting('app.matrix_authority_owner',true) AND product=current_setting('app.matrix_authority_product',true));
CREATE FUNCTION matrix_storage_root_consent_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'matrix root consent is retained' USING ERRCODE='55000'; END IF;
 IF current_setting('app.tenant',true) IS DISTINCT FROM NEW.tenant
 OR current_setting('app.matrix_authority_owner',true) IS DISTINCT FROM NEW.subject
 OR current_setting('app.matrix_authority_product',true) IS DISTINCT FROM NEW.product
 OR coalesce(current_setting('app.merge',true),'') NOT IN('','false','off','0') THEN
  RAISE EXCEPTION 'matrix root consent scope mismatch' USING ERRCODE='42501';
 END IF;
 IF TG_OP='UPDATE' AND (
 (OLD.tenant,OLD.subject,OLD.product,OLD.nonce,OLD.action,OLD.request,OLD.created_at,OLD.expires_at) IS DISTINCT FROM (NEW.tenant,NEW.subject,NEW.product,NEW.nonce,NEW.action,NEW.request,NEW.created_at,NEW.expires_at)
 OR OLD.consumed_at IS NOT NULL OR NEW.consumed_at IS NULL
 ) THEN RAISE EXCEPTION 'matrix root consent is immutable or already consumed' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER matrix_storage_root_consent_guard BEFORE INSERT OR UPDATE OR DELETE ON matrix_storage_root_consents FOR EACH ROW EXECUTE FUNCTION matrix_storage_root_consent_guard();
CREATE FUNCTION matrix_storage_authority_requires_history() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
 old_tenant text := coalesce(current_setting('app.tenant',true),'');
 old_owner text := coalesce(current_setting('app.matrix_authority_owner',true),'');
 old_product text := coalesce(current_setting('app.matrix_authority_product',true),'');
BEGIN
 -- The write guard already bound NEW to its owner. A lifecycle transaction may
 -- touch several scopes; its deferred audit reads only this exact NEW scope.
 PERFORM set_config('app.tenant',NEW.tenant,true),set_config('app.matrix_authority_owner',NEW.subject,true),set_config('app.matrix_authority_product',NEW.product,true);
 IF NOT EXISTS(SELECT 1 FROM matrix_storage_authority_history h WHERE h.tenant=NEW.tenant AND h.subject=NEW.subject AND h.product=NEW.product AND h.generation=NEW.generation AND h.master_key=NEW.master_key AND h.status=NEW.status) THEN
  RAISE EXCEPTION 'matrix authority transition requires retained evidence' USING ERRCODE='55000';
 END IF;
 PERFORM set_config('app.tenant',old_tenant,true),set_config('app.matrix_authority_owner',old_owner,true),set_config('app.matrix_authority_product',old_product,true);
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER matrix_storage_authority_requires_history AFTER INSERT OR UPDATE ON matrix_storage_authorities DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION matrix_storage_authority_requires_history();
