-- SPDX-License-Identifier: Apache-2.0
-- Public SDK authorization and opaque origin commitments only. No content,
-- provider credential, user access token, claim token or recovery material.
CREATE TABLE matrix_storage_publications (
 tenant text NOT NULL,
 subject text NOT NULL,
 product text NOT NULL,
 id uuid NOT NULL,
 namespace text NOT NULL,
 origin_kind text NOT NULL CHECK(origin_kind IN('signed','legacy')),
 matrix_user_id text NOT NULL,
 device_id text NOT NULL,
 room_id text NOT NULL CHECK(left(room_id,1)='!'),
 transaction_id text NOT NULL CHECK(octet_length(transaction_id) BETWEEN 1 AND 255),
 marker_id text,
 cipher_sha256 text NOT NULL CHECK(cipher_sha256 ~ '^[0-9a-f]{64}$'),
 request_sha256 text,
 authority_generation bigint NOT NULL CHECK(authority_generation>=0),
 request jsonb,
 producer_evidence jsonb,
 status text NOT NULL CHECK(status IN('reserved','sending','accepted','cancelled')),
 event_id text,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 accepted_at timestamptz,
 PRIMARY KEY(tenant,subject,product,id),
 UNIQUE(tenant,subject,product,room_id,transaction_id),
 UNIQUE(marker_id),
 UNIQUE(event_id),
 CHECK(namespace='storage:'||product),
 CHECK(
  (origin_kind='signed' AND device_id<>'' AND marker_id IS NOT NULL AND request_sha256 IS NOT NULL
   AND request IS NOT NULL AND producer_evidence IS NOT NULL
   AND marker_id ~ '^[A-Za-z0-9_-]{42}[AEIMQUYcgkosw048]$'
   AND request_sha256 ~ '^[0-9a-f]{64}$' AND authority_generation>0
   AND jsonb_typeof(request)='object' AND octet_length(request::text)<=16384
   AND jsonb_typeof(producer_evidence)='object' AND octet_length(producer_evidence::text)<=32768)
  OR
  (origin_kind='legacy' AND device_id='' AND marker_id IS NULL
   AND request_sha256 IS NULL AND request IS NULL AND producer_evidence IS NULL)
 ),
 CHECK(
  (status IN('reserved','cancelled') AND event_id IS NULL AND accepted_at IS NULL)
  OR (status='sending' AND event_id IS NOT NULL AND left(event_id,1)='$' AND accepted_at IS NULL)
  OR (status='accepted' AND event_id IS NOT NULL AND left(event_id,1)='$' AND accepted_at IS NOT NULL)
 ),
 CHECK(origin_kind<>'legacy' OR status IN('sending','accepted'))
);
CREATE INDEX matrix_storage_publications_pending
 ON matrix_storage_publications(tenant,subject,product,device_id)
 WHERE status='sending';
ALTER TABLE matrix_storage_publications ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_storage_publications FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_storage_publications_owner ON matrix_storage_publications
 USING(tenant=current_setting('app.tenant',true)
  AND subject=current_setting('app.matrix_authority_owner',true)
  AND product=current_setting('app.matrix_authority_product',true))
 WITH CHECK(tenant=current_setting('app.tenant',true)
  AND subject=current_setting('app.matrix_authority_owner',true)
  AND product=current_setting('app.matrix_authority_product',true));
CREATE POLICY matrix_storage_publications_contract ON matrix_storage_publications AS RESTRICTIVE
 USING(current_setting('app.matrix_publication_contract',true)='1')
 WITH CHECK(current_setting('app.matrix_publication_contract',true)='1');

CREATE FUNCTION matrix_storage_publication_guard() RETURNS trigger
 LANGUAGE plpgsql SECURITY INVOKER SET search_path=pg_catalog,public AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  RAISE EXCEPTION 'publication evidence is retained' USING ERRCODE='55000';
 END IF;
 IF current_setting('app.matrix_publication_contract',true) IS DISTINCT FROM '1'
 OR current_setting('app.tenant',true) IS DISTINCT FROM NEW.tenant
 OR current_setting('app.matrix_authority_owner',true) IS DISTINCT FROM NEW.subject
 OR current_setting('app.matrix_authority_product',true) IS DISTINCT FROM NEW.product THEN
  RAISE EXCEPTION 'publication scope mismatch' USING ERRCODE='42501';
 END IF;
 IF TG_OP='INSERT' AND (
  (NEW.origin_kind='signed' AND NEW.status<>'reserved')
  OR (NEW.origin_kind='legacy' AND NEW.status<>'sending')
 ) THEN RAISE EXCEPTION 'invalid initial publication state' USING ERRCODE='55000'; END IF;
 IF TG_OP='UPDATE' AND (
  (OLD.tenant,OLD.subject,OLD.product,OLD.id,OLD.namespace,OLD.origin_kind,OLD.matrix_user_id,
   OLD.device_id,OLD.room_id,OLD.transaction_id,OLD.marker_id,OLD.cipher_sha256,
   OLD.request_sha256,OLD.authority_generation,OLD.request,OLD.producer_evidence,OLD.created_at)
  IS DISTINCT FROM
  (NEW.tenant,NEW.subject,NEW.product,NEW.id,NEW.namespace,NEW.origin_kind,NEW.matrix_user_id,
   NEW.device_id,NEW.room_id,NEW.transaction_id,NEW.marker_id,NEW.cipher_sha256,
   NEW.request_sha256,NEW.authority_generation,NEW.request,NEW.producer_evidence,NEW.created_at)
  OR NOT (
   (OLD.status='reserved' AND NEW.status IN('sending','cancelled'))
   OR (OLD.status='sending' AND NEW.status='accepted' AND OLD.event_id=NEW.event_id)
  )
 ) THEN RAISE EXCEPTION 'publication evidence is immutable' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER matrix_storage_publication_guard BEFORE INSERT OR UPDATE OR DELETE
 ON matrix_storage_publications FOR EACH ROW EXECUTE FUNCTION matrix_storage_publication_guard();

-- Old fence writers cannot hide pending origin operations by omitting the new
-- RLS contract. Reserved writes are cancelled atomically when a fence is planned.
CREATE FUNCTION matrix_storage_publication_fence_guard() RETURNS trigger
 LANGUAGE plpgsql SECURITY INVOKER SET search_path=pg_catalog,public AS $$
DECLARE previous_contract text; pending boolean;
BEGIN
 previous_contract:=current_setting('app.matrix_publication_contract',true);
 PERFORM set_config('app.matrix_publication_contract','1',true);
 IF TG_OP='INSERT' THEN
  UPDATE public.matrix_storage_publications SET status='cancelled'
   WHERE tenant=NEW.tenant AND subject=NEW.subject AND product=NEW.product
    AND device_id=NEW.fence#>>'{previous,target,device_id}' AND status='reserved';
 ELSIF OLD.status='planned' AND NEW.status='confirmed' THEN
  SELECT EXISTS(SELECT 1 FROM public.matrix_storage_publications
   WHERE tenant=NEW.tenant AND subject=NEW.subject AND product=NEW.product
    AND (device_id=NEW.fence#>>'{previous,target,device_id}' OR origin_kind='legacy')
    AND status='sending') INTO pending;
  PERFORM set_config('app.matrix_publication_contract',COALESCE(previous_contract,''),true);
  IF pending THEN RAISE EXCEPTION 'publication origin is still pending' USING ERRCODE='55000'; END IF;
  RETURN NEW;
 END IF;
 PERFORM set_config('app.matrix_publication_contract',COALESCE(previous_contract,''),true);
 RETURN NEW;
END $$;
CREATE TRIGGER matrix_storage_publication_fence_guard BEFORE INSERT OR UPDATE
 ON matrix_storage_device_fences FOR EACH ROW EXECUTE FUNCTION matrix_storage_publication_fence_guard();
SELECT sytue_scope_merge_register('matrix_storage_publications'::regclass);
SELECT sytue_scope_admission_register('matrix_storage_publications'::regclass,false);
