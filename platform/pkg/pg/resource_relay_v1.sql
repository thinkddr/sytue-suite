-- SPDX-License-Identifier: Apache-2.0
-- Public routing/CAS only. Application data, management credentials and epoch
-- private identities never belong here. Signature/pin verification uses resource.
CREATE FUNCTION sytue_resource_e2e_ciphertext_only(raw bytea) RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
 SELECT coalesce(jsonb_typeof(j)='object' AND j->>'algorithm'='m.megolm.v1.aes-sha2'
  AND jsonb_typeof(j->'ciphertext')='string' AND length(j->>'ciphertext')>0
  AND jsonb_typeof(j->'session_id')='string' AND length(j->>'session_id')>0
  AND NOT(j ?| ARRAY['body','msgtype','type','content']),false)
 FROM (SELECT convert_from(raw,'UTF8')::jsonb AS j)s
$$;
CREATE TABLE resource_e2e_resources(
 tenant text NOT NULL, product text NOT NULL CHECK(product<>''),
 kind text NOT NULL, resource_id text NOT NULL,
 epoch text NOT NULL,generation bigint NOT NULL CHECK(generation>0),
 policy_sha256 text NOT NULL CHECK(policy_sha256 ~ '^[a-f0-9]{64}$'),
 membership_sha256 text NOT NULL CHECK(membership_sha256 ~ '^[a-f0-9]{64}$'),
 private_rules boolean NOT NULL,rules_version text NOT NULL CHECK(rules_version ~ '^[a-f0-9]{64}$'),
 baseline_seq bigint NOT NULL CHECK(baseline_seq>=0),baseline_head_cipher_sha256 text NOT NULL,
 seq bigint NOT NULL CHECK(seq>=baseline_seq),head_cipher_sha256 text NOT NULL,
 state text NOT NULL DEFAULT 'active' CHECK(state IN('active','draining','revoked')),
 hold_op_id text,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant,product,kind,resource_id),
 CHECK((baseline_seq=0 AND (baseline_head_cipher_sha256='' OR baseline_head_cipher_sha256 ~ '^[a-f0-9]{64}$')) OR(baseline_seq>0 AND baseline_head_cipher_sha256 ~ '^[a-f0-9]{64}$')),
 CHECK((seq=0 AND (head_cipher_sha256='' OR head_cipher_sha256 ~ '^[a-f0-9]{64}$')) OR(seq>0 AND head_cipher_sha256 ~ '^[a-f0-9]{64}$')),
 CHECK(kind<>'' AND resource_id<>'' AND epoch<>'')
);
CREATE TABLE resource_e2e_epochs(
 tenant text NOT NULL,product text NOT NULL,kind text NOT NULL,resource_id text NOT NULL,
 epoch text NOT NULL,generation bigint NOT NULL CHECK(generation>0),
 policy_sha256 text NOT NULL CHECK(policy_sha256 ~ '^[a-f0-9]{64}$'),
 signed_policy bytea NOT NULL CHECK(octet_length(signed_policy) BETWEEN 2 AND 262144),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant,product,kind,resource_id,epoch),
 UNIQUE(tenant,product,kind,resource_id,generation),
 UNIQUE(tenant,product,kind,resource_id,policy_sha256),
 FOREIGN KEY(tenant,product,kind,resource_id) REFERENCES resource_e2e_resources(tenant,product,kind,resource_id) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE resource_e2e_public_evidence(
 tenant text NOT NULL,product text NOT NULL CHECK(product<>''),
 sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),
 value bytea NOT NULL CHECK(octet_length(value)>0 AND sha256=encode(digest(value,'sha256'),'hex')),
 created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(tenant,product,sha256)
);
CREATE TABLE resource_e2e_candidates(
 tenant text NOT NULL,product text NOT NULL,kind text NOT NULL,resource_id text NOT NULL,
 op_id text NOT NULL,epoch text NOT NULL,generation bigint NOT NULL CHECK(generation>0),
 publication_sha256 text NOT NULL CHECK(publication_sha256 ~ '^[a-f0-9]{64}$'),
 candidate_sha256 text NOT NULL CHECK(candidate_sha256 ~ '^[a-f0-9]{64}$'),
 reservation_sha256 text CHECK(reservation_sha256 ~ '^[a-f0-9]{64}$'),reservation bytea,
 CHECK((reservation IS NULL AND reservation_sha256 IS NULL) OR(reservation IS NOT NULL AND reservation_sha256 IS NOT NULL AND octet_length(reservation)>0 AND reservation_sha256=encode(digest(reservation,'sha256'),'hex'))),
 signed_publication bytea NOT NULL CHECK(octet_length(signed_publication) BETWEEN 2 AND 262144),
 session_certificate bytea NOT NULL CHECK(octet_length(session_certificate) BETWEEN 2 AND 262144),
 encrypted_event bytea NOT NULL CHECK(octet_length(encrypted_event) BETWEEN 2 AND 262144 AND sytue_resource_e2e_ciphertext_only(encrypted_event)),
 resolution bytea CHECK(resolution IS NULL OR octet_length(resolution) BETWEEN 2 AND 262144),
 validation_certificate bytea CHECK(validation_certificate IS NULL OR octet_length(validation_certificate) BETWEEN 2 AND 262144),
 base_seq bigint NOT NULL CHECK(base_seq>=0),base_head_cipher_sha256 text NOT NULL,
 op_count bigint NOT NULL CHECK(op_count>0 AND base_seq<=9223372036854775807-op_count),
 outer_cipher_sha256 text NOT NULL CHECK(outer_cipher_sha256 ~ '^[a-f0-9]{64}$'),
 attempt_id uuid NOT NULL,
 CHECK(coalesce(attempt_id::text=(convert_from(signed_publication,'UTF8')::jsonb)->>'transaction_id',false)),
 state text NOT NULL CHECK(state IN('waiting_validation','prepared','sending','ambiguous','committed','stale','cancelled')),
 CHECK(state NOT IN('sending','ambiguous','committed') OR reservation IS NOT NULL),
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant,product,kind,resource_id,op_id),
 UNIQUE(attempt_id),
 FOREIGN KEY(tenant,product,kind,resource_id,epoch) REFERENCES resource_e2e_epochs(tenant,product,kind,resource_id,epoch),
 CHECK((base_seq=0 AND (base_head_cipher_sha256='' OR base_head_cipher_sha256 ~ '^[a-f0-9]{64}$')) OR(base_seq>0 AND base_head_cipher_sha256 ~ '^[a-f0-9]{64}$'))
);
CREATE TABLE resource_e2e_receipts(
 tenant text NOT NULL,product text NOT NULL,kind text NOT NULL,resource_id text NOT NULL,
 op_id text NOT NULL,publication_sha256 text NOT NULL CHECK(publication_sha256 ~ '^[a-f0-9]{64}$'),
 epoch text NOT NULL,base_seq bigint NOT NULL CHECK(base_seq>=0),seq bigint NOT NULL CHECK(seq>base_seq),
 base_head_cipher_sha256 text NOT NULL,head_cipher_sha256 text NOT NULL CHECK(head_cipher_sha256 ~ '^[a-f0-9]{64}$'),
 room_id text NOT NULL,event_id text NOT NULL,
 UNIQUE(tenant,product,kind,resource_id,room_id,event_id),
 receipt bytea NOT NULL CHECK(octet_length(receipt) BETWEEN 2 AND 262144),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant,product,kind,resource_id,op_id),
 UNIQUE(tenant,product,kind,resource_id,base_seq),
 UNIQUE(tenant,product,kind,resource_id,seq),
 FOREIGN KEY(tenant,product,kind,resource_id,op_id) REFERENCES resource_e2e_candidates(tenant,product,kind,resource_id,op_id),
 CHECK((base_seq=0 AND (base_head_cipher_sha256='' OR base_head_cipher_sha256 ~ '^[a-f0-9]{64}$')) OR(base_seq>0 AND base_head_cipher_sha256 ~ '^[a-f0-9]{64}$'))
);
ALTER TABLE resource_e2e_resources ADD CONSTRAINT resource_e2e_hold_fk FOREIGN KEY(tenant,product,kind,resource_id,hold_op_id) REFERENCES resource_e2e_candidates(tenant,product,kind,resource_id,op_id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE resource_e2e_resources ADD CONSTRAINT resource_e2e_epoch_fk FOREIGN KEY(tenant,product,kind,resource_id,epoch) REFERENCES resource_e2e_epochs(tenant,product,kind,resource_id,epoch) DEFERRABLE INITIALLY DEFERRED;
CREATE INDEX resource_e2e_pending ON resource_e2e_candidates(tenant,product,kind,resource_id,created_at) WHERE state IN('waiting_validation','prepared','sending','ambiguous');
DO $rls$
DECLARE relation text;
BEGIN
 FOREACH relation IN ARRAY ARRAY['resource_e2e_resources','resource_e2e_epochs','resource_e2e_candidates','resource_e2e_receipts','resource_e2e_public_evidence'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',relation);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',relation);
  EXECUTE format($policy$CREATE POLICY resource_e2e_tenant ON %I USING(tenant=current_setting('app.tenant',true)) WITH CHECK(tenant=current_setting('app.tenant',true))$policy$,relation);
 END LOOP;
END $rls$;

CREATE FUNCTION sytue_resource_e2e_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'encrypted resource history is immutable' USING ERRCODE='PST02';END
$$;
CREATE TRIGGER resource_e2e_evidence_immutable BEFORE UPDATE OR DELETE ON resource_e2e_public_evidence FOR EACH ROW EXECUTE FUNCTION sytue_resource_e2e_immutable();
CREATE TRIGGER resource_e2e_epoch_immutable BEFORE UPDATE OR DELETE ON resource_e2e_epochs FOR EACH ROW EXECUTE FUNCTION sytue_resource_e2e_immutable();
CREATE TRIGGER resource_e2e_receipt_immutable BEFORE UPDATE OR DELETE ON resource_e2e_receipts FOR EACH ROW EXECUTE FUNCTION sytue_resource_e2e_immutable();
CREATE FUNCTION sytue_resource_e2e_candidate_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR
  (NEW.tenant,NEW.product,NEW.kind,NEW.resource_id,NEW.op_id,NEW.epoch,NEW.generation,NEW.publication_sha256,NEW.candidate_sha256,NEW.signed_publication,NEW.session_certificate,NEW.encrypted_event,NEW.base_seq,NEW.base_head_cipher_sha256,NEW.op_count,NEW.outer_cipher_sha256,NEW.attempt_id,NEW.created_at)
  IS DISTINCT FROM
  (OLD.tenant,OLD.product,OLD.kind,OLD.resource_id,OLD.op_id,OLD.epoch,OLD.generation,OLD.publication_sha256,OLD.candidate_sha256,OLD.signed_publication,OLD.session_certificate,OLD.encrypted_event,OLD.base_seq,OLD.base_head_cipher_sha256,OLD.op_count,OLD.outer_cipher_sha256,OLD.attempt_id,OLD.created_at) THEN
  RAISE EXCEPTION 'encrypted candidate identity is immutable' USING ERRCODE='PST02';
 END IF;
 IF OLD.reservation IS NOT NULL AND (NEW.reservation,NEW.reservation_sha256) IS DISTINCT FROM (OLD.reservation,OLD.reservation_sha256) THEN RAISE EXCEPTION 'captured reservation is immutable' USING ERRCODE='PST02';END IF;
 IF OLD.reservation IS NULL AND NEW.reservation IS NOT NULL AND NOT(OLD.state='prepared' AND NEW.state='sending') THEN RAISE EXCEPTION 'reservation is captured only at sending acquire' USING ERRCODE='PST02';END IF;
 IF NEW.state IN('sending','ambiguous','committed') AND NEW.reservation IS NULL THEN RAISE EXCEPTION 'sending requires a captured public reservation' USING ERRCODE='PST02';END IF;
 IF OLD.validation_certificate IS NOT NULL AND NEW.validation_certificate IS DISTINCT FROM OLD.validation_certificate THEN RAISE EXCEPTION 'validation certificate is immutable' USING ERRCODE='PST02';END IF;
 IF NEW.state<>OLD.state AND NOT(
  (OLD.state='waiting_validation' AND NEW.state IN('prepared','stale','cancelled')) OR
  (OLD.state='prepared' AND NEW.state IN('sending','stale','cancelled')) OR
  (OLD.state='sending' AND NEW.state IN('ambiguous','committed','cancelled')) OR
  (OLD.state='ambiguous' AND NEW.state IN('committed','cancelled'))
 ) THEN RAISE EXCEPTION 'invalid durable candidate transition' USING ERRCODE='PST02';END IF;
 IF OLD.state IN('sending','ambiguous') AND NEW.state='cancelled' AND NEW.resolution IS NULL THEN RAISE EXCEPTION 'send cancellation requires durable reconciliation evidence' USING ERRCODE='PST02';END IF;
 IF NEW.state='committed' AND NOT EXISTS(SELECT 1 FROM resource_e2e_receipts r WHERE (r.tenant,r.product,r.kind,r.resource_id,r.op_id)=(OLD.tenant,OLD.product,OLD.kind,OLD.resource_id,OLD.op_id) AND r.publication_sha256=OLD.publication_sha256) THEN RAISE EXCEPTION 'candidate commit requires accepted receipt' USING ERRCODE='PST02';END IF;
 IF OLD.state IN('committed','cancelled','stale') AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'terminal candidate is immutable' USING ERRCODE='PST02';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER resource_e2e_candidate_guard BEFORE UPDATE OR DELETE ON resource_e2e_candidates FOR EACH ROW EXECUTE FUNCTION sytue_resource_e2e_candidate_guard();
CREATE FUNCTION sytue_resource_e2e_receipt_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM resource_e2e_resources r JOIN resource_e2e_candidates c ON (c.tenant,c.product,c.kind,c.resource_id,c.op_id)=(r.tenant,r.product,r.kind,r.resource_id,r.hold_op_id)
  WHERE (r.tenant,r.product,r.kind,r.resource_id)=(NEW.tenant,NEW.product,NEW.kind,NEW.resource_id)
   AND c.op_id=NEW.op_id AND c.state IN('sending','ambiguous') AND c.generation=r.generation AND c.epoch=r.epoch AND c.epoch=NEW.epoch
   AND c.publication_sha256=NEW.publication_sha256 AND c.base_seq=r.seq AND c.base_seq=NEW.base_seq AND NEW.seq=c.base_seq+c.op_count
   AND c.base_head_cipher_sha256=r.head_cipher_sha256 AND c.base_head_cipher_sha256=NEW.base_head_cipher_sha256
   AND c.outer_cipher_sha256=NEW.head_cipher_sha256 AND NEW.room_id=(convert_from(c.signed_publication,'UTF8')::jsonb)->>'room_id'
   AND (NOT r.private_rules OR c.validation_certificate IS NOT NULL)) THEN
  RAISE EXCEPTION 'receipt does not resolve the current sending fence' USING ERRCODE='PST02';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER resource_e2e_receipt_guard BEFORE INSERT ON resource_e2e_receipts FOR EACH ROW EXECUTE FUNCTION sytue_resource_e2e_receipt_guard();
CREATE FUNCTION sytue_resource_e2e_resource_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'encrypted resource routing is retained' USING ERRCODE='PST02';END IF;
 IF (NEW.tenant,NEW.product,NEW.kind,NEW.resource_id,NEW.baseline_seq,NEW.baseline_head_cipher_sha256,NEW.created_at) IS DISTINCT FROM (OLD.tenant,OLD.product,OLD.kind,OLD.resource_id,OLD.baseline_seq,OLD.baseline_head_cipher_sha256,OLD.created_at) THEN RAISE EXCEPTION 'resource identity is immutable' USING ERRCODE='PST02';END IF;
 IF NEW.seq<OLD.seq OR NEW.generation<OLD.generation THEN RAISE EXCEPTION 'resource cannot rewind' USING ERRCODE='PST02';END IF;
 IF OLD.state='revoked' AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'revoked resource is immutable' USING ERRCODE='PST02';END IF;
 IF (NEW.epoch,NEW.generation,NEW.policy_sha256,NEW.membership_sha256,NEW.private_rules,NEW.rules_version) IS DISTINCT FROM (OLD.epoch,OLD.generation,OLD.policy_sha256,OLD.membership_sha256,OLD.private_rules,OLD.rules_version) THEN
  IF OLD.hold_op_id IS NOT NULL OR OLD.state<>'draining' OR NEW.generation<>OLD.generation+1 OR NEW.epoch=OLD.epoch OR NEW.hold_op_id IS NOT NULL THEN RAISE EXCEPTION 'resource epoch requires a drained fence' USING ERRCODE='PST02';END IF;
 END IF;
 IF OLD.hold_op_id IS NOT NULL AND NEW.state='revoked' THEN RAISE EXCEPTION 'unresolved send blocks revocation cutover' USING ERRCODE='PST02';END IF;
 IF OLD.hold_op_id IS NOT NULL AND NEW.hold_op_id IS NOT NULL AND NEW.hold_op_id<>OLD.hold_op_id THEN RAISE EXCEPTION 'unresolved send cannot be replaced' USING ERRCODE='PST02';END IF;
 IF NEW.hold_op_id IS NOT NULL AND OLD.hold_op_id IS NULL THEN
  IF OLD.state<>'active' OR NOT EXISTS(SELECT 1 FROM resource_e2e_candidates c WHERE (c.tenant,c.product,c.kind,c.resource_id,c.op_id)=(OLD.tenant,OLD.product,OLD.kind,OLD.resource_id,NEW.hold_op_id) AND c.state='sending' AND c.reservation IS NOT NULL AND c.generation=OLD.generation AND c.epoch=OLD.epoch AND c.base_seq=OLD.seq AND c.base_head_cipher_sha256=OLD.head_cipher_sha256 AND (NOT OLD.private_rules OR c.validation_certificate IS NOT NULL)) THEN RAISE EXCEPTION 'invalid sending hold' USING ERRCODE='PST02';END IF;
 END IF;
 IF OLD.hold_op_id IS NOT NULL AND NEW.hold_op_id IS NULL AND NOT EXISTS(SELECT 1 FROM resource_e2e_candidates c WHERE (c.tenant,c.product,c.kind,c.resource_id,c.op_id)=(OLD.tenant,OLD.product,OLD.kind,OLD.resource_id,OLD.hold_op_id) AND c.state IN('committed','cancelled')) THEN RAISE EXCEPTION 'ambiguous send must be reconciled' USING ERRCODE='PST02';END IF;
 IF (NEW.seq,NEW.head_cipher_sha256) IS DISTINCT FROM (OLD.seq,OLD.head_cipher_sha256) THEN
  IF OLD.hold_op_id IS NULL OR NEW.hold_op_id IS NOT NULL OR NOT EXISTS(SELECT 1 FROM resource_e2e_receipts r WHERE (r.tenant,r.product,r.kind,r.resource_id,r.op_id)=(OLD.tenant,OLD.product,OLD.kind,OLD.resource_id,OLD.hold_op_id) AND r.base_seq=OLD.seq AND r.base_head_cipher_sha256=OLD.head_cipher_sha256 AND r.seq=NEW.seq AND r.head_cipher_sha256=NEW.head_cipher_sha256) THEN RAISE EXCEPTION 'head requires an accepted durable receipt' USING ERRCODE='PST02';END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER resource_e2e_resource_guard BEFORE UPDATE OR DELETE ON resource_e2e_resources FOR EACH ROW EXECUTE FUNCTION sytue_resource_e2e_resource_guard();
DO $immutable$
DECLARE relation text;
BEGIN
 FOREACH relation IN ARRAY ARRAY['resource_e2e_resources','resource_e2e_epochs','resource_e2e_candidates','resource_e2e_receipts','resource_e2e_public_evidence'] LOOP
  EXECUTE format('CREATE TRIGGER resource_e2e_no_truncate BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION sytue_resource_e2e_immutable()',relation);
 END LOOP;
END $immutable$;
