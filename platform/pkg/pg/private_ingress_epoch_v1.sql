-- An epoch is allocated once for the whole ingress scope. Key-plan RLS remains
-- epoch-specific; this separate public binding permits an owner to detect an
-- already retained epoch without exposing another epoch's key plans or tasks.
CREATE TABLE private_ingress_scopes (
 tenant text NOT NULL, owner_sub text NOT NULL, product text NOT NULL,
 epoch uuid NOT NULL,
 PRIMARY KEY (tenant,owner_sub,product),
 UNIQUE (tenant,owner_sub,product,epoch)
);

-- A migration owns this table lock for the complete transaction. Backfill sees
-- every retained plan even for a FORCE-RLS owner, and restores the original RLS
-- before commit. Ambiguous old history is retained and blocks this upgrade.
LOCK TABLE private_ingress_key_plans IN ACCESS EXCLUSIVE MODE;
ALTER TABLE private_ingress_key_plans DISABLE ROW LEVEL SECURITY;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM private_ingress_key_plans GROUP BY tenant,owner_sub,product HAVING count(DISTINCT epoch)>1) THEN
  RAISE EXCEPTION 'private ingress epochs require explicit reconciliation' USING ERRCODE='PIG01';
 END IF;
END $$;
INSERT INTO private_ingress_scopes(tenant,owner_sub,product,epoch)
 SELECT DISTINCT tenant,owner_sub,product,epoch FROM private_ingress_key_plans;
ALTER TABLE private_ingress_key_plans ADD CONSTRAINT private_ingress_epoch_scope
 FOREIGN KEY (tenant,owner_sub,product,epoch)
 REFERENCES private_ingress_scopes(tenant,owner_sub,product,epoch);
ALTER TABLE private_ingress_key_plans ENABLE ROW LEVEL SECURITY;
ALTER TABLE private_ingress_key_plans FORCE ROW LEVEL SECURITY;

ALTER TABLE private_ingress_scopes ENABLE ROW LEVEL SECURITY;
ALTER TABLE private_ingress_scopes FORCE ROW LEVEL SECURITY;
CREATE POLICY private_ingress_scope_owner ON private_ingress_scopes
 USING (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.private_ingress_owner',true) AND product=current_setting('app.private_ingress_product',true))
 WITH CHECK (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.private_ingress_owner',true) AND product=current_setting('app.private_ingress_product',true) AND epoch::text=current_setting('app.private_ingress_epoch',true));
CREATE TRIGGER private_ingress_scope BEFORE INSERT OR UPDATE OR DELETE ON private_ingress_scopes
 FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_scope_guard();
SELECT sytue_scope_merge_register('private_ingress_scopes'::regclass);
SELECT sytue_scope_admission_register('private_ingress_scopes'::regclass);

CREATE FUNCTION sytue_private_ingress_epoch_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
DECLARE accepted uuid;
BEGIN
 INSERT INTO private_ingress_scopes(tenant,owner_sub,product,epoch)
 VALUES(NEW.tenant,NEW.owner_sub,NEW.product,NEW.epoch)
 ON CONFLICT(tenant,owner_sub,product) DO NOTHING;
 SELECT epoch INTO STRICT accepted FROM private_ingress_scopes
 WHERE tenant=NEW.tenant AND owner_sub=NEW.owner_sub AND product=NEW.product FOR SHARE;
 IF accepted IS DISTINCT FROM NEW.epoch THEN
  RAISE EXCEPTION 'private ingress epoch is already retained' USING ERRCODE='PIG01';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER private_ingress_epoch BEFORE INSERT ON private_ingress_key_plans
 FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_epoch_guard();
