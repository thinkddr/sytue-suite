-- SPDX-License-Identifier: Apache-2.0
-- Opaque age bodies for small Calendar ingress. No ICS, envelope or plaintext
-- digest is retained. Row scope/task and ciphertext commitments are immutable.
CREATE TABLE private_ingress_bodies (
 tenant text NOT NULL,owner_sub text NOT NULL,product text NOT NULL,epoch uuid NOT NULL,
 id text NOT NULL CHECK(id ~ '^[A-Za-z0-9_-]{1,128}$'),
 task_id text NOT NULL CHECK(task_id ~ '^[A-Za-z0-9_-]{1,128}$'),
 ciphertext bytea NOT NULL CHECK(octet_length(ciphertext)>0),
 cipher_sha256 text NOT NULL CHECK(cipher_sha256=encode(sha256(ciphertext),'hex')),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant,owner_sub,product,epoch,id),
 UNIQUE(tenant,owner_sub,product,epoch,task_id),
 FOREIGN KEY(tenant,owner_sub,product,epoch,task_id) REFERENCES private_ingress_tasks(tenant,owner_sub,product,epoch,id)
);
ALTER TABLE private_ingress_bodies ENABLE ROW LEVEL SECURITY;
ALTER TABLE private_ingress_bodies FORCE ROW LEVEL SECURITY;
CREATE POLICY private_ingress_bodies_owner ON private_ingress_bodies
 USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.private_ingress_owner',true) AND product=current_setting('app.private_ingress_product',true) AND epoch::text=current_setting('app.private_ingress_epoch',true))
 WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.private_ingress_owner',true) AND product=current_setting('app.private_ingress_product',true) AND epoch::text=current_setting('app.private_ingress_epoch',true));
CREATE TRIGGER private_ingress_scope BEFORE INSERT OR UPDATE OR DELETE ON private_ingress_bodies FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_scope_guard();
CREATE FUNCTION sytue_private_ingress_body_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'private ingress ciphertext is immutable and retained' USING ERRCODE='55000'; END $$;
CREATE TRIGGER private_ingress_body_immutable BEFORE UPDATE OR DELETE ON private_ingress_bodies FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_body_guard();
SELECT sytue_scope_merge_register('private_ingress_bodies'::regclass);
