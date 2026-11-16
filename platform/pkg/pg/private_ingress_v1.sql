-- Shared opaque ingress metadata. Domain payloads and plaintext digests MUST
-- remain in the encrypted blob; this schema holds public routing only.
CREATE TABLE IF NOT EXISTS private_ingress_key_plans (
 tenant text NOT NULL, owner_sub text NOT NULL, product text NOT NULL,
 epoch uuid NOT NULL, key_generation bigint NOT NULL CHECK(key_generation>0 AND key_generation<9007199254740992),
 key_id text NOT NULL CHECK(key_id ~ '^[0-9a-f]{64}$'),
 profile text NOT NULL CHECK(profile='age-X25519-v1'),
 public_recipient text NOT NULL CHECK(octet_length(public_recipient) BETWEEN 1 AND 256),
 created_at timestamptz NOT NULL DEFAULT now(), revoked_at timestamptz,
 PRIMARY KEY(tenant,owner_sub,product,epoch,key_generation),
 UNIQUE(key_id),
 CHECK(key_id=encode(sha256(convert_to(public_recipient,'UTF8')),'hex'))
);

CREATE TABLE IF NOT EXISTS private_ingress_devices (
 tenant text NOT NULL, owner_sub text NOT NULL, product text NOT NULL, epoch uuid NOT NULL,
 device_id text NOT NULL CHECK(octet_length(device_id) BETWEEN 1 AND 255),
 room_id text NOT NULL CHECK(room_id LIKE '!%' AND octet_length(room_id) BETWEEN 2 AND 255),
 signing_key text NOT NULL CHECK(octet_length(signing_key) BETWEEN 1 AND 128),
 authorization_generation bigint NOT NULL DEFAULT 1 CHECK(authorization_generation>0 AND authorization_generation<9007199254740992),
 enrolled_at timestamptz NOT NULL DEFAULT now(), revoked_at timestamptz, fenced_at timestamptz,
 PRIMARY KEY(tenant,owner_sub,product,epoch,device_id),
 CHECK(fenced_at IS NULL OR revoked_at IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS private_ingress_tasks (
 tenant text NOT NULL, owner_sub text NOT NULL, product text NOT NULL, epoch uuid NOT NULL,
 id text NOT NULL CHECK(id ~ '^[A-Za-z0-9_-]{1,128}$'), kind text NOT NULL,
 key_generation bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 blob_id text CHECK(blob_id ~ '^[A-Za-z0-9_-]{1,128}$'),
 cipher_size bigint CHECK(cipher_size>0 AND cipher_size<=140737488355328),
 cipher_sha256 text CHECK(cipher_sha256 ~ '^[0-9a-f]{64}$'),
 device_id text, room_id text,
 claim_generation bigint NOT NULL DEFAULT 0 CHECK(claim_generation>=0 AND claim_generation<9007199254740992),
 claim_token_sha256 bytea, lease_until timestamptz,
 fence_required_at timestamptz,
 receipt_room text, receipt_event text, acknowledged_at timestamptz,
 PRIMARY KEY(tenant,owner_sub,product,epoch,id),
 FOREIGN KEY(tenant,owner_sub,product,epoch,key_generation) REFERENCES private_ingress_key_plans(tenant,owner_sub,product,epoch,key_generation),
 FOREIGN KEY(tenant,owner_sub,product,epoch,device_id) REFERENCES private_ingress_devices(tenant,owner_sub,product,epoch,device_id),
 CHECK((product='mail' AND kind='mail.receive') OR (product='calendar' AND kind='calendar.receive')),
 CHECK((blob_id IS NULL AND cipher_size IS NULL AND cipher_sha256 IS NULL) OR (blob_id IS NOT NULL AND cipher_size IS NOT NULL AND cipher_sha256 IS NOT NULL)),
 CHECK((device_id IS NULL AND room_id IS NULL AND claim_generation=0 AND claim_token_sha256 IS NULL AND lease_until IS NULL) OR (device_id IS NOT NULL AND room_id LIKE '!%' AND claim_generation>0 AND octet_length(claim_token_sha256)=32 AND lease_until IS NOT NULL AND blob_id IS NOT NULL)),
 CHECK((acknowledged_at IS NULL AND receipt_room IS NULL AND receipt_event IS NULL) OR (acknowledged_at IS NOT NULL AND receipt_room=room_id AND receipt_event LIKE '$%' AND cipher_sha256 IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS private_ingress_tasks_pending ON private_ingress_tasks(tenant,owner_sub,product,epoch,created_at,id) WHERE acknowledged_at IS NULL AND cipher_sha256 IS NOT NULL;

CREATE OR REPLACE FUNCTION sytue_private_ingress_scope_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'private ingress history is retained' USING ERRCODE='55000'; END IF;
 IF coalesce(current_setting('app.maintenance',true),'') NOT IN('','false','off','0') OR coalesce(current_setting('app.merge',true),'') NOT IN('','false','off','0') THEN
  RAISE EXCEPTION 'private ingress disallows maintenance and merge' USING ERRCODE='42501';
 END IF;
 IF current_setting('app.tenant',true) IS DISTINCT FROM NEW.tenant
 OR current_setting('app.private_ingress_owner',true) IS DISTINCT FROM NEW.owner_sub
 OR current_setting('app.private_ingress_product',true) IS DISTINCT FROM NEW.product
 OR current_setting('app.private_ingress_epoch',true) IS DISTINCT FROM NEW.epoch::text THEN
  RAISE EXCEPTION 'private ingress scope mismatch' USING ERRCODE='42501';
 END IF;
 IF TG_OP='UPDATE' AND (OLD.tenant,OLD.owner_sub,OLD.product,OLD.epoch) IS DISTINCT FROM (NEW.tenant,NEW.owner_sub,NEW.product,NEW.epoch) THEN
  RAISE EXCEPTION 'private ingress scope is immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION sytue_private_ingress_device_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (OLD.device_id,OLD.room_id,OLD.signing_key,OLD.enrolled_at) IS DISTINCT FROM (NEW.device_id,NEW.room_id,NEW.signing_key,NEW.enrolled_at)
 OR (OLD.revoked_at IS NOT NULL AND OLD.revoked_at IS DISTINCT FROM NEW.revoked_at)
 OR (OLD.fenced_at IS NOT NULL AND OLD.fenced_at IS DISTINCT FROM NEW.fenced_at)
 OR NEW.authorization_generation<OLD.authorization_generation
 OR (OLD.revoked_at IS NULL AND NEW.revoked_at IS NOT NULL AND NEW.authorization_generation<>OLD.authorization_generation+1) THEN
  RAISE EXCEPTION 'private ingress device authorization is immutable or monotonic' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION sytue_private_ingress_key_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (OLD.key_generation,OLD.key_id,OLD.profile,OLD.public_recipient,OLD.created_at) IS DISTINCT FROM (NEW.key_generation,NEW.key_id,NEW.profile,NEW.public_recipient,NEW.created_at)
 OR (OLD.revoked_at IS NOT NULL AND OLD.revoked_at IS DISTINCT FROM NEW.revoked_at) THEN
  RAISE EXCEPTION 'private ingress key plan is immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION sytue_private_ingress_task_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (OLD.id,OLD.kind,OLD.key_generation,OLD.created_at) IS DISTINCT FROM (NEW.id,NEW.kind,NEW.key_generation,NEW.created_at)
 OR (OLD.cipher_sha256 IS NOT NULL AND (OLD.blob_id,OLD.cipher_size,OLD.cipher_sha256) IS DISTINCT FROM (NEW.blob_id,NEW.cipher_size,NEW.cipher_sha256))
 OR (OLD.acknowledged_at IS NOT NULL AND OLD IS DISTINCT FROM NEW) THEN
  RAISE EXCEPTION 'private ingress delivery and receipt are immutable' USING ERRCODE='55000';
 END IF;
 IF OLD.device_id IS NOT NULL AND (OLD.device_id,OLD.room_id,OLD.claim_generation,OLD.claim_token_sha256) IS DISTINCT FROM (NEW.device_id,NEW.room_id,NEW.claim_generation,NEW.claim_token_sha256) AND (NEW.claim_generation<>OLD.claim_generation+1 OR NOT EXISTS(
  SELECT 1 FROM private_ingress_devices d WHERE d.tenant=OLD.tenant AND d.owner_sub=OLD.owner_sub AND d.product=OLD.product AND d.epoch=OLD.epoch AND d.device_id=OLD.device_id AND d.fenced_at IS NOT NULL
 )) THEN
  RAISE EXCEPTION 'private ingress handoff requires a verified device fence' USING ERRCODE='42501';
 END IF;
 RETURN NEW;
END $$;

DO $$ DECLARE tab text; BEGIN
 FOREACH tab IN ARRAY ARRAY['private_ingress_key_plans','private_ingress_devices','private_ingress_tasks'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',tab);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',tab);
  IF NOT EXISTS(SELECT 1 FROM pg_policies WHERE schemaname=current_schema() AND tablename=tab AND policyname='private_ingress_owner') THEN
   EXECUTE format('CREATE POLICY private_ingress_owner ON %I USING (tenant=current_setting(''app.tenant'',true) AND owner_sub=current_setting(''app.private_ingress_owner'',true) AND product=current_setting(''app.private_ingress_product'',true) AND epoch::text=current_setting(''app.private_ingress_epoch'',true)) WITH CHECK (tenant=current_setting(''app.tenant'',true) AND owner_sub=current_setting(''app.private_ingress_owner'',true) AND product=current_setting(''app.private_ingress_product'',true) AND epoch::text=current_setting(''app.private_ingress_epoch'',true))',tab);
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid=tab::regclass AND tgname='private_ingress_scope') THEN
   EXECUTE format('CREATE TRIGGER private_ingress_scope BEFORE INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_scope_guard()',tab);
  END IF;
 END LOOP;
END $$;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='private_ingress_key_plans'::regclass AND tgname='private_ingress_key_immutable') THEN
  CREATE TRIGGER private_ingress_key_immutable BEFORE UPDATE ON private_ingress_key_plans FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_key_guard();
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='private_ingress_devices'::regclass AND tgname='private_ingress_device_immutable') THEN
  CREATE TRIGGER private_ingress_device_immutable BEFORE UPDATE ON private_ingress_devices FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_device_guard();
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='private_ingress_tasks'::regclass AND tgname='private_ingress_task_immutable') THEN
  CREATE TRIGGER private_ingress_task_immutable BEFORE UPDATE ON private_ingress_tasks FOR EACH ROW EXECUTE FUNCTION sytue_private_ingress_task_guard();
 END IF;
END $$;
