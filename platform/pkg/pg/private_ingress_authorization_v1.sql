-- SPDX-License-Identifier: Apache-2.0
-- Nonces and signed public transcripts stay in the owning domain database.
-- Claim tokens and application payloads are never retained in this table.
CREATE TABLE IF NOT EXISTS private_ingress_actions (
 tenant text NOT NULL,owner_sub text NOT NULL,product text NOT NULL,epoch uuid NOT NULL,
 nonce uuid NOT NULL,
 action jsonb NOT NULL CHECK(jsonb_typeof(action)='object' AND octet_length(action::text)<=16384),
 created_at timestamptz NOT NULL DEFAULT now(),expires_at timestamptz NOT NULL,consumed_at timestamptz,
 PRIMARY KEY(tenant,owner_sub,product,epoch,nonce),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '6 minutes')
);
CREATE INDEX IF NOT EXISTS private_ingress_actions_pending ON private_ingress_actions(tenant,owner_sub,product,epoch,expires_at) WHERE consumed_at IS NULL;
ALTER TABLE private_ingress_actions ENABLE ROW LEVEL SECURITY;
ALTER TABLE private_ingress_actions FORCE ROW LEVEL SECURITY;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_policies WHERE schemaname=current_schema() AND tablename='private_ingress_actions' AND policyname='private_ingress_owner') THEN
  CREATE POLICY private_ingress_owner ON private_ingress_actions
  USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.private_ingress_owner',true) AND product=current_setting('app.private_ingress_product',true) AND epoch::text=current_setting('app.private_ingress_epoch',true))
  WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.private_ingress_owner',true) AND product=current_setting('app.private_ingress_product',true) AND epoch::text=current_setting('app.private_ingress_epoch',true));
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='private_ingress_actions'::regclass AND tgname='private_ingress_scope') THEN
  CREATE TRIGGER private_ingress_scope BEFORE INSERT OR UPDATE OR DELETE ON private_ingress_actions FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_scope_guard();
 END IF;
END $$;
CREATE OR REPLACE FUNCTION sytue_private_ingress_action_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (OLD.tenant,OLD.owner_sub,OLD.product,OLD.epoch,OLD.nonce,OLD.action,OLD.created_at,OLD.expires_at) IS DISTINCT FROM (NEW.tenant,NEW.owner_sub,NEW.product,NEW.epoch,NEW.nonce,NEW.action,NEW.created_at,NEW.expires_at)
 OR OLD.consumed_at IS NOT NULL OR NEW.consumed_at IS NULL THEN
  RAISE EXCEPTION 'private ingress action is immutable or already consumed' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='private_ingress_actions'::regclass AND tgname='private_ingress_action_immutable') THEN
  CREATE TRIGGER private_ingress_action_immutable BEFORE UPDATE ON private_ingress_actions FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_action_guard();
 END IF;
END $$;
