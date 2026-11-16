-- SPDX-License-Identifier: Apache-2.0
-- Original claims are retained before a different device can be fenced/granted.
-- Only public routes, ciphertext commitments and hashes are durable here.
CREATE TABLE private_ingress_claim_history(
 tenant text NOT NULL,owner_sub text NOT NULL,product text NOT NULL,epoch uuid NOT NULL,
 task_id text NOT NULL,claim_generation bigint NOT NULL CHECK(claim_generation>0 AND claim_generation<9007199254740992),
 device_id text NOT NULL,room_id text NOT NULL,signing_key text NOT NULL CHECK(octet_length(signing_key)=43),
 claim_token_sha256 bytea NOT NULL CHECK(octet_length(claim_token_sha256)=32),lease_until timestamptz NOT NULL,
 receipt_room text,receipt_event text,acknowledged_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant,owner_sub,product,epoch,task_id,claim_generation),
 FOREIGN KEY(tenant,owner_sub,product,epoch,task_id) REFERENCES private_ingress_tasks(tenant,owner_sub,product,epoch,id),
 CHECK((receipt_room IS NULL AND receipt_event IS NULL AND acknowledged_at IS NULL) OR (receipt_room IS NOT NULL AND receipt_event IS NOT NULL AND acknowledged_at IS NOT NULL))
);
CREATE TABLE private_ingress_handoffs(
 tenant text NOT NULL,owner_sub text NOT NULL,product text NOT NULL,epoch uuid NOT NULL,id uuid NOT NULL,
 task_id text NOT NULL,previous_generation bigint NOT NULL,
 target_device_id text NOT NULL,target_room_id text NOT NULL,token_sha256 bytea NOT NULL CHECK(octet_length(token_sha256)=32),
 authority_generation bigint NOT NULL CHECK(authority_generation>0 AND authority_generation<9007199254740992),
 authority_user_id text NOT NULL,authority_root_key text NOT NULL CHECK(octet_length(authority_root_key)=43),
 request_sha256 text NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
 resolve_request_sha256 text CHECK(resolve_request_sha256 ~ '^[0-9a-f]{64}$'),
 status text NOT NULL CHECK(status IN('fencing','readable','claimed','acknowledged')),
 fence jsonb CHECK(fence IS NULL OR (jsonb_typeof(fence)='object' AND octet_length(fence::text)<=16384 AND fence->>'status'='confirmed')),
 resolved_receipt jsonb CHECK(resolved_receipt IS NULL OR (jsonb_typeof(resolved_receipt)='object' AND octet_length(resolved_receipt::text)<=8192)),
 new_claim_generation bigint,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant,owner_sub,product,epoch,id),
 UNIQUE(tenant,owner_sub,product,epoch,task_id,previous_generation),
 FOREIGN KEY(tenant,owner_sub,product,epoch,task_id,previous_generation) REFERENCES private_ingress_claim_history(tenant,owner_sub,product,epoch,task_id,claim_generation),
 FOREIGN KEY(tenant,owner_sub,product,epoch,target_device_id) REFERENCES private_ingress_devices(tenant,owner_sub,product,epoch,device_id),
 CHECK((status='fencing' AND fence IS NULL AND resolved_receipt IS NULL AND new_claim_generation IS NULL AND resolve_request_sha256 IS NULL)
 OR (status='readable' AND (fence IS NOT NULL OR resolved_receipt IS NOT NULL) AND new_claim_generation IS NULL AND resolve_request_sha256 IS NULL)
 OR (status='claimed' AND fence IS NOT NULL AND resolved_receipt IS NULL AND new_claim_generation=previous_generation+1 AND resolve_request_sha256 IS NOT NULL)
 OR (status='acknowledged' AND resolved_receipt IS NOT NULL AND resolve_request_sha256 IS NOT NULL AND (new_claim_generation IS NULL OR new_claim_generation=previous_generation+1)))
);
CREATE FUNCTION sytue_private_ingress_claim_history_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'original private ingress claim must be retained' USING ERRCODE='55000'; END $$;
CREATE TRIGGER private_ingress_claim_history_immutable BEFORE UPDATE OR DELETE ON private_ingress_claim_history FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_claim_history_guard();
CREATE FUNCTION sytue_private_ingress_handoff_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'private ingress handoff must be retained' USING ERRCODE='55000'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.status NOT IN('fencing','readable') OR NEW.fence IS NOT NULL THEN RAISE EXCEPTION 'handoff must start before remote side effects' USING ERRCODE='55000'; END IF;
  RETURN NEW;
 END IF;
 IF (OLD.tenant,OLD.owner_sub,OLD.product,OLD.epoch,OLD.id,OLD.task_id,OLD.previous_generation,OLD.target_device_id,OLD.target_room_id,OLD.token_sha256,OLD.authority_generation,OLD.authority_user_id,OLD.authority_root_key,OLD.request_sha256,OLD.created_at)
 IS DISTINCT FROM (NEW.tenant,NEW.owner_sub,NEW.product,NEW.epoch,NEW.id,NEW.task_id,NEW.previous_generation,NEW.target_device_id,NEW.target_room_id,NEW.token_sha256,NEW.authority_generation,NEW.authority_user_id,NEW.authority_root_key,NEW.request_sha256,NEW.created_at)
 OR (OLD.fence IS NOT NULL AND OLD.fence IS DISTINCT FROM NEW.fence)
 OR (OLD.resolved_receipt IS NOT NULL AND OLD.resolved_receipt IS DISTINCT FROM NEW.resolved_receipt)
 OR (OLD.resolve_request_sha256 IS NOT NULL AND OLD.resolve_request_sha256 IS DISTINCT FROM NEW.resolve_request_sha256)
 OR OLD.status='acknowledged'
 OR NOT ((OLD.status='fencing' AND NEW.status='readable') OR (OLD.status='readable' AND NEW.status IN('claimed','acknowledged')) OR (OLD.status='claimed' AND NEW.status='acknowledged')) THEN
  RAISE EXCEPTION 'private ingress handoff is immutable or monotonic' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER private_ingress_handoff_immutable BEFORE INSERT OR UPDATE OR DELETE ON private_ingress_handoffs FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_handoff_guard();
-- A device fence by itself is not a transferable claim. The exact retained
-- original and consented destination must still be in the read-only phase.
CREATE FUNCTION sytue_private_ingress_handoff_task_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.device_id IS NOT NULL AND (OLD.device_id,OLD.room_id,OLD.claim_generation,OLD.claim_token_sha256)
 IS DISTINCT FROM (NEW.device_id,NEW.room_id,NEW.claim_generation,NEW.claim_token_sha256)
 AND NOT EXISTS(
  SELECT 1 FROM private_ingress_handoffs h JOIN private_ingress_claim_history c
  ON (c.tenant,c.owner_sub,c.product,c.epoch,c.task_id,c.claim_generation)=(h.tenant,h.owner_sub,h.product,h.epoch,h.task_id,h.previous_generation)
  WHERE (h.tenant,h.owner_sub,h.product,h.epoch,h.task_id,h.previous_generation)=(OLD.tenant,OLD.owner_sub,OLD.product,OLD.epoch,OLD.id,OLD.claim_generation)
  AND (c.device_id,c.room_id,c.claim_token_sha256,c.lease_until)=(OLD.device_id,OLD.room_id,OLD.claim_token_sha256,OLD.lease_until)
  AND h.status='claimed' AND h.resolve_request_sha256 IS NOT NULL AND h.new_claim_generation=NEW.claim_generation AND h.fence->>'status'='confirmed'
  AND (h.target_device_id,h.target_room_id,h.token_sha256)=(NEW.device_id,NEW.room_id,NEW.claim_token_sha256)
  AND NEW.claim_generation=OLD.claim_generation+1
 ) THEN RAISE EXCEPTION 'private ingress reassignment requires retained reconciliation' USING ERRCODE='42501';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER private_ingress_handoff_task BEFORE UPDATE ON private_ingress_tasks FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_handoff_task_guard();
DO $$ DECLARE tab text; BEGIN
 FOREACH tab IN ARRAY ARRAY['private_ingress_claim_history','private_ingress_handoffs'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',tab);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',tab);
  EXECUTE format('CREATE POLICY private_ingress_owner ON %I USING (tenant=current_setting(''app.tenant'',true) AND owner_sub=current_setting(''app.private_ingress_owner'',true) AND product=current_setting(''app.private_ingress_product'',true) AND epoch::text=current_setting(''app.private_ingress_epoch'',true)) WITH CHECK (tenant=current_setting(''app.tenant'',true) AND owner_sub=current_setting(''app.private_ingress_owner'',true) AND product=current_setting(''app.private_ingress_product'',true) AND epoch::text=current_setting(''app.private_ingress_epoch'',true))',tab);
  EXECUTE format('CREATE TRIGGER private_ingress_scope BEFORE INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_scope_guard()',tab);
  PERFORM sytue_scope_merge_register(tab::regclass);
  PERFORM sytue_scope_admission_register(tab::regclass);
 END LOOP;
END $$;
