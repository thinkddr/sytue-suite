-- SPDX-License-Identifier: Apache-2.0
-- A rejected begin ID cannot later start a fence, including with a new nonce.
CREATE TABLE private_ingress_handoff_rejections(
 tenant text NOT NULL,owner_sub text NOT NULL,product text NOT NULL,epoch uuid NOT NULL,
 id uuid NOT NULL,task_id text NOT NULL,
 rejection jsonb NOT NULL CHECK(jsonb_typeof(rejection)='object' AND octet_length(rejection::text)<=16384 AND rejection->>'reason'='assignment_changed'),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant,owner_sub,product,epoch,id),
 FOREIGN KEY(tenant,owner_sub,product,epoch,task_id) REFERENCES private_ingress_tasks(tenant,owner_sub,product,epoch,id)
);
ALTER TABLE private_ingress_handoff_rejections ENABLE ROW LEVEL SECURITY;
ALTER TABLE private_ingress_handoff_rejections FORCE ROW LEVEL SECURITY;
CREATE POLICY private_ingress_owner ON private_ingress_handoff_rejections
 USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.private_ingress_owner',true) AND product=current_setting('app.private_ingress_product',true) AND epoch::text=current_setting('app.private_ingress_epoch',true))
 WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.private_ingress_owner',true) AND product=current_setting('app.private_ingress_product',true) AND epoch::text=current_setting('app.private_ingress_epoch',true));
CREATE TRIGGER private_ingress_scope BEFORE INSERT OR UPDATE OR DELETE ON private_ingress_handoff_rejections FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_scope_guard();
CREATE FUNCTION sytue_private_ingress_rejection_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'private ingress rejection must be retained' USING ERRCODE='55000'; END IF;
 PERFORM 1 FROM private_ingress_tasks t WHERE (t.tenant,t.owner_sub,t.product,t.epoch,t.id)=(NEW.tenant,NEW.owner_sub,NEW.product,NEW.epoch,NEW.task_id) FOR UPDATE;
 IF EXISTS(SELECT 1 FROM private_ingress_handoffs h WHERE (h.tenant,h.owner_sub,h.product,h.epoch,h.id)=(NEW.tenant,NEW.owner_sub,NEW.product,NEW.epoch,NEW.id)) THEN
  RAISE EXCEPTION 'started handoff cannot be rejected' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER private_ingress_rejection_immutable BEFORE INSERT OR UPDATE OR DELETE ON private_ingress_handoff_rejections FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_rejection_guard();
CREATE FUNCTION sytue_private_ingress_rejection_closes_begin() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM 1 FROM private_ingress_tasks t WHERE (t.tenant,t.owner_sub,t.product,t.epoch,t.id)=(NEW.tenant,NEW.owner_sub,NEW.product,NEW.epoch,NEW.task_id) FOR UPDATE;
 IF EXISTS(SELECT 1 FROM private_ingress_handoff_rejections r WHERE (r.tenant,r.owner_sub,r.product,r.epoch,r.id)=(NEW.tenant,NEW.owner_sub,NEW.product,NEW.epoch,NEW.id)) THEN
  RAISE EXCEPTION 'rejected handoff cannot start' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER private_ingress_rejected_begin BEFORE INSERT ON private_ingress_handoffs FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_rejection_closes_begin();
SELECT sytue_scope_merge_register('private_ingress_handoff_rejections'::regclass);
SELECT sytue_scope_admission_register('private_ingress_handoff_rejections'::regclass);
