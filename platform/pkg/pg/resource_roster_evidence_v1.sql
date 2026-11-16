-- SPDX-License-Identifier: Apache-2.0
-- Historical PUBLIC signer evidence is captured before a pending upload can
-- outlive its creator device. Old pre-evidence rows remain retained, not trusted.
ALTER TABLE resource_e2e_roster_captures
 ADD COLUMN evidence_sha256 text,
 ADD COLUMN evidence bytea,
 ADD COLUMN creator_signing_key text,
 ADD COLUMN creator_encryption_key text,
 ADD COLUMN root_generation bigint,
 ADD CONSTRAINT resource_e2e_roster_evidence_shape CHECK(
 (evidence_sha256 IS NULL AND evidence IS NULL AND creator_signing_key IS NULL AND creator_encryption_key IS NULL AND root_generation IS NULL)
 OR(evidence_sha256 IS NOT NULL AND evidence IS NOT NULL AND creator_signing_key IS NOT NULL AND creator_encryption_key IS NOT NULL AND root_generation>0
 AND evidence_sha256=encode(digest(evidence,'sha256'),'hex')
 AND coalesce((convert_from(evidence,'UTF8')::jsonb)->>'v'='1',false)
 AND coalesce((convert_from(evidence,'UTF8')::jsonb)->>'domain'='org.sytue.resource.bootstrap.capture-evidence.v1',false)
 AND coalesce((convert_from(evidence,'UTF8')::jsonb)->>'plan_sha256'=plan_sha256,false)
 AND coalesce((convert_from(evidence,'UTF8')::jsonb)->>'manifest_sha256'=manifest_sha256,false)
 AND coalesce((convert_from(evidence,'UTF8')::jsonb)->>'session_binding_sha256'=session_binding_sha256,false)
 AND coalesce((convert_from(evidence,'UTF8')::jsonb)#>>'{plan,signer,pin,SigningKey}'=creator_signing_key,false)
 AND coalesce((convert_from(evidence,'UTF8')::jsonb)#>>'{plan,signer,pin,EncryptionKey}'=creator_encryption_key,false)
 AND coalesce((convert_from(evidence,'UTF8')::jsonb)#>'{plan,signer,pin}'=(convert_from(evidence,'UTF8')::jsonb)#>'{manifest,signer,pin}',false))
 );
CREATE FUNCTION sytue_resource_e2e_roster_evidence_required()RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.evidence IS NULL OR NEW.evidence_sha256 IS NULL OR NEW.root_generation IS NULL THEN RAISE EXCEPTION 'new capture requires original current SDK evidence' USING ERRCODE='PST02';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER resource_e2e_roster_capture_evidence_required BEFORE INSERT ON resource_e2e_roster_captures FOR EACH ROW EXECUTE FUNCTION sytue_resource_e2e_roster_evidence_required();

-- The SAME capture lock fences Finalize/BeginBootstrap against metadata-only
-- cancellation. There is no TTL, absence-of-HTTP claim or release of a send hold.
CREATE FUNCTION sytue_resource_e2e_roster_bootstrap_fence()RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE capture_row record;
BEGIN
 SELECT * INTO capture_row FROM resource_e2e_roster_captures c
 WHERE(c.tenant,c.product,c.kind,c.resource_id)=(NEW.tenant,NEW.product,NEW.kind,NEW.resource_id)
 ORDER BY(c.state<>'cancelled')DESC,c.created_at DESC,c.plan_id DESC LIMIT 1 FOR UPDATE;
 IF capture_row.plan_id IS NOT NULL AND(capture_row.state='cancelled'OR capture_row.plan_id<>NEW.plan_id OR capture_row.plan_sha256<>NEW.plan_sha256 OR capture_row.received_count<>capture_row.member_count)THEN RAISE EXCEPTION 'bootstrap cannot escape original pending capture fence' USING ERRCODE='PST02';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER resource_e2e_roster_bootstrap_fence BEFORE INSERT ON resource_e2e_bootstraps FOR EACH ROW EXECUTE FUNCTION sytue_resource_e2e_roster_bootstrap_fence();
