-- SPDX-License-Identifier: Apache-2.0
-- Public handoff evidence only: no claim token, recovery key or private content.
CREATE TABLE matrix_storage_device_fences (
 tenant text NOT NULL,
 subject text NOT NULL,
 product text NOT NULL,
 epoch uuid NOT NULL,
 id uuid NOT NULL,
 request_sha256 text NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
 evidence jsonb NOT NULL CHECK(jsonb_typeof(evidence)='object' AND octet_length(evidence::text)<=32768),
 fence jsonb NOT NULL CHECK(jsonb_typeof(fence)='object' AND octet_length(fence::text)<=16384),
 status text NOT NULL DEFAULT 'planned' CHECK(status IN('planned','confirmed')),
 confirmed_at timestamptz,
 PRIMARY KEY(tenant,subject,product,epoch,id),
 FOREIGN KEY(tenant,subject,product) REFERENCES matrix_storage_authorities(tenant,subject,product),
 CHECK((status='planned' AND confirmed_at IS NULL) OR (status='confirmed' AND confirmed_at IS NOT NULL))
);
ALTER TABLE matrix_storage_device_fences ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_storage_device_fences FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_storage_device_fences_owner ON matrix_storage_device_fences
 USING(tenant=current_setting('app.tenant',true) AND subject=current_setting('app.matrix_authority_owner',true) AND product=current_setting('app.matrix_authority_product',true))
 WITH CHECK(tenant=current_setting('app.tenant',true) AND subject=current_setting('app.matrix_authority_owner',true) AND product=current_setting('app.matrix_authority_product',true));
CREATE FUNCTION matrix_storage_device_fence_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'storage device fence is retained' USING ERRCODE='55000'; END IF;
 IF current_setting('app.tenant',true) IS DISTINCT FROM NEW.tenant
 OR current_setting('app.matrix_authority_owner',true) IS DISTINCT FROM NEW.subject
 OR current_setting('app.matrix_authority_product',true) IS DISTINCT FROM NEW.product THEN
  RAISE EXCEPTION 'storage device fence scope mismatch' USING ERRCODE='42501';
 END IF;
 IF TG_OP='INSERT' AND (NEW.status<>'planned' OR NEW.confirmed_at IS NOT NULL) THEN
  RAISE EXCEPTION 'storage device fence must start planned' USING ERRCODE='55000';
 END IF;
 IF TG_OP='UPDATE' AND (
  (OLD.tenant,OLD.subject,OLD.product,OLD.epoch,OLD.id,OLD.request_sha256,OLD.fence,OLD.evidence) IS DISTINCT FROM
  (NEW.tenant,NEW.subject,NEW.product,NEW.epoch,NEW.id,NEW.request_sha256,NEW.fence,NEW.evidence)
  OR OLD.status<>'planned' OR NEW.status<>'confirmed' OR OLD.confirmed_at IS NOT NULL OR NEW.confirmed_at IS NULL
 ) THEN RAISE EXCEPTION 'storage device fence evidence is immutable' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER matrix_storage_device_fence_guard BEFORE INSERT OR UPDATE OR DELETE ON matrix_storage_device_fences
 FOR EACH ROW EXECUTE FUNCTION matrix_storage_device_fence_guard();
SELECT sytue_scope_merge_register('matrix_storage_device_fences'::regclass);
SELECT sytue_scope_admission_register('matrix_storage_device_fences'::regclass,false);
