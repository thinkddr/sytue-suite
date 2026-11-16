-- SPDX-License-Identifier: Apache-2.0
-- Pre-epoch preparation belongs to the same resource relay. No fake policy or
-- accepted reference is needed before the first real baseline/checkpoint ACK.
CREATE TABLE resource_e2e_bootstraps(
 tenant text NOT NULL,product text NOT NULL,kind text NOT NULL,resource_id text NOT NULL,
 plan_id uuid NOT NULL UNIQUE,room_id text NOT NULL UNIQUE CHECK(left(room_id,1)='!'),
 epoch text NOT NULL CHECK(epoch<>''),source_epoch uuid NOT NULL,
 source_history text NOT NULL DEFAULT 'full' CHECK(source_history='full'),
 source_seq bigint NOT NULL CHECK(source_seq>=0),acl_version bigint NOT NULL CHECK(acl_version>=0),
 owner_sub text NOT NULL,creator_user text NOT NULL,creator_device text NOT NULL,creator_root text NOT NULL,
 session_generation bigint NOT NULL CHECK(session_generation>0),
 session_binding_sha256 text NOT NULL CHECK(session_binding_sha256~'^[a-f0-9]{64}$'),
 plan_sha256 text NOT NULL CHECK(plan_sha256=encode(digest(plan,'sha256'),'hex')),
 plan bytea NOT NULL,signed_plan bytea NOT NULL,
 evidence_sha256 text NOT NULL CHECK(evidence_sha256=encode(digest(evidence,'sha256'),'hex')),
 evidence bytea NOT NULL,
 state text NOT NULL DEFAULT 'preparing' CHECK(state IN('preparing','active')),
 created_at timestamptz NOT NULL DEFAULT now(),activated_at timestamptz,
 PRIMARY KEY(tenant,product,kind,resource_id),
 UNIQUE(tenant,product,kind,resource_id,plan_id),
 CHECK((state='preparing' AND activated_at IS NULL)OR(state='active' AND activated_at IS NOT NULL)),
 CHECK((convert_from(signed_plan,'UTF8')::jsonb)-'signatures'=convert_from(plan,'UTF8')::jsonb),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,resource,tenant}'=tenant,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,resource,product}'=product,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,resource,kind}'=kind,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,resource,id}'=resource_id,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)->>'id'=plan_id::text,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)->>'room_id'=room_id,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)->>'epoch'=epoch,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,source_epoch}'=source_epoch::text,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,history}'=source_history,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)->>'history'=source_history,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,source_seq}'=source_seq::text,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,acl_version}'=acl_version::text,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{creator_scope,subject}'=owner_sub,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{creator,user_id}'=creator_user,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{creator,device_id}'=creator_device,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{creator,root_key}'=creator_root,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)->>'session_binding_sha256'=session_binding_sha256,false))
);
CREATE TABLE resource_e2e_bootstrap_slots(
 tenant text NOT NULL,product text NOT NULL,kind text NOT NULL,resource_id text NOT NULL,plan_id uuid NOT NULL,
 bootstrap_kind text NOT NULL CHECK(bootstrap_kind IN('baseline','checkpoint')),
 attempt_id uuid NOT NULL UNIQUE,
 candidate_sha256 text NOT NULL CHECK(candidate_sha256~'^[a-f0-9]{64}$'),
 context_sha256 text NOT NULL CHECK(context_sha256=encode(digest(context,'sha256'),'hex')),context bytea NOT NULL,
 outer_cipher_sha256 text NOT NULL CHECK(outer_cipher_sha256~'^[a-f0-9]{64}$'),
 signed_publication bytea NOT NULL,session_certificate bytea NOT NULL,
 encrypted_event bytea NOT NULL CHECK(sytue_resource_e2e_ciphertext_only(encrypted_event)),
 state text NOT NULL CHECK(state IN('prepared','sending','ambiguous','accepted')),
 room_id text,event_id text,created_at timestamptz NOT NULL DEFAULT now(),accepted_at timestamptz,
 PRIMARY KEY(tenant,product,kind,resource_id,plan_id,bootstrap_kind),
 FOREIGN KEY(tenant,product,kind,resource_id,plan_id)REFERENCES resource_e2e_bootstraps(tenant,product,kind,resource_id,plan_id),
 CHECK(coalesce((convert_from(signed_publication,'UTF8')::jsonb)->>'transaction_id'=attempt_id::text,false)),
 CHECK(coalesce((convert_from(signed_publication,'UTF8')::jsonb)->>'kind'=bootstrap_kind,false)),
 CHECK(coalesce((convert_from(signed_publication,'UTF8')::jsonb)->>'context_sha256'=context_sha256,false)),
 CHECK(coalesce((convert_from(signed_publication,'UTF8')::jsonb)->>'outer_cipher_sha256'=outer_cipher_sha256,false)),
 CHECK((state IN('prepared','sending','ambiguous')AND room_id IS NULL AND event_id IS NULL AND accepted_at IS NULL)
 OR(state='accepted' AND room_id IS NOT NULL AND left(room_id,1)='!' AND event_id IS NOT NULL AND left(event_id,1)='$' AND accepted_at IS NOT NULL))
);
DO $$ DECLARE relation text; BEGIN
 FOREACH relation IN ARRAY ARRAY['resource_e2e_bootstraps','resource_e2e_bootstrap_slots'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',relation);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',relation);
  EXECUTE format($policy$CREATE POLICY resource_e2e_tenant ON %I USING(tenant=current_setting('app.tenant',true))WITH CHECK(tenant=current_setting('app.tenant',true))$policy$,relation);
  EXECUTE format('CREATE TRIGGER resource_e2e_no_truncate BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION sytue_resource_e2e_immutable()',relation);
 END LOOP;
END $$;
CREATE FUNCTION sytue_resource_e2e_bootstrap_guard()RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE e jsonb;b record;c record;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'bootstrap evidence is retained' USING ERRCODE='PST02';END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'preparing' OR EXISTS(SELECT 1 FROM resource_e2e_resources r WHERE(r.tenant,r.product,r.kind,r.resource_id)=(NEW.tenant,NEW.product,NEW.kind,NEW.resource_id))THEN RAISE EXCEPTION 'bootstrap already initialized' USING ERRCODE='PST02';END IF;
  RETURN NEW;
 END IF;
 IF(to_jsonb(OLD)-'state'-'activated_at')IS DISTINCT FROM(to_jsonb(NEW)-'state'-'activated_at')OR OLD.state<>'preparing' OR NEW.state<>'active' THEN RAISE EXCEPTION 'bootstrap plan is immutable' USING ERRCODE='PST02';END IF;
 SELECT convert_from(p.signed_policy,'UTF8')::jsonb INTO e FROM resource_e2e_resources r JOIN resource_e2e_epochs p USING(tenant,product,kind,resource_id,epoch)WHERE(r.tenant,r.product,r.kind,r.resource_id)=(NEW.tenant,NEW.product,NEW.kind,NEW.resource_id)AND r.epoch=NEW.epoch AND r.generation=1 AND r.state='active';
 SELECT * INTO b FROM resource_e2e_bootstrap_slots s WHERE(s.tenant,s.product,s.kind,s.resource_id,s.plan_id)=(NEW.tenant,NEW.product,NEW.kind,NEW.resource_id,NEW.plan_id)AND s.bootstrap_kind='baseline' AND s.state='accepted';
 SELECT * INTO c FROM resource_e2e_bootstrap_slots s WHERE(s.tenant,s.product,s.kind,s.resource_id,s.plan_id)=(NEW.tenant,NEW.product,NEW.kind,NEW.resource_id,NEW.plan_id)AND s.bootstrap_kind='checkpoint' AND s.state='accepted';
 IF e IS NULL OR b.event_id IS NULL OR c.event_id IS NULL OR b.event_id=c.event_id
 OR e#>>'{baseline,reference,room_id}'IS DISTINCT FROM NEW.room_id OR e#>>'{baseline,reference,event_id}'IS DISTINCT FROM b.event_id
 OR e#>>'{baseline,position,seq}'IS DISTINCT FROM NEW.source_seq::text OR e#>>'{baseline,position,head_cipher_sha256}'IS DISTINCT FROM b.outer_cipher_sha256
 OR e#>>'{baseline,context_sha256}'IS DISTINCT FROM b.context_sha256
 OR e#>>'{checkpoint,reference,room_id}'IS DISTINCT FROM NEW.room_id OR e#>>'{checkpoint,reference,event_id}'IS DISTINCT FROM c.event_id
 OR e#>>'{checkpoint,cipher_sha256}'IS DISTINCT FROM c.outer_cipher_sha256 OR e#>>'{checkpoint,context_sha256}'IS DISTINCT FROM c.context_sha256
 THEN RAISE EXCEPTION 'bootstrap requires both exact accepted SDK publications' USING ERRCODE='PST02';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER resource_e2e_bootstrap_guard BEFORE INSERT OR UPDATE OR DELETE ON resource_e2e_bootstraps FOR EACH ROW EXECUTE FUNCTION sytue_resource_e2e_bootstrap_guard();
CREATE FUNCTION sytue_resource_e2e_bootstrap_slot_guard()RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p record;b record;j jsonb;
BEGIN
 IF TG_OP='DELETE'THEN RAISE EXCEPTION 'bootstrap slot is retained' USING ERRCODE='PST02';END IF;
 SELECT * INTO p FROM resource_e2e_bootstraps WHERE(tenant,product,kind,resource_id,plan_id)=(NEW.tenant,NEW.product,NEW.kind,NEW.resource_id,NEW.plan_id)FOR UPDATE;
 IF p.plan_id IS NULL THEN RAISE EXCEPTION 'bootstrap plan missing' USING ERRCODE='PST02';END IF;
 IF TG_OP='INSERT'THEN
  j:=convert_from(NEW.context,'UTF8')::jsonb;
  IF p.state<>'preparing' OR NEW.state<>'prepared' OR(convert_from(NEW.signed_publication,'UTF8')::jsonb)->>'plan_sha256'IS DISTINCT FROM p.plan_sha256
  OR j->>'kind'IS DISTINCT FROM NEW.bootstrap_kind OR j->>'room_id'IS DISTINCT FROM p.room_id OR j->>'epoch'IS DISTINCT FROM p.epoch
  OR j#>>'{source_cutover,seq}'IS DISTINCT FROM p.source_seq::text
  THEN RAISE EXCEPTION 'bootstrap context/plan mismatch' USING ERRCODE='PST02';END IF;
  IF NEW.bootstrap_kind='baseline'THEN
   IF j#>>'{source_cutover,head_cipher_sha256}'IS DISTINCT FROM ''THEN RAISE EXCEPTION 'baseline has no invented pre-cipher head' USING ERRCODE='PST02';END IF;
  ELSE
   SELECT * INTO b FROM resource_e2e_bootstrap_slots s WHERE(s.tenant,s.product,s.kind,s.resource_id,s.plan_id)=(NEW.tenant,NEW.product,NEW.kind,NEW.resource_id,NEW.plan_id)AND s.bootstrap_kind='baseline' AND s.state='accepted';
   IF b.event_id IS NULL OR j#>>'{source_cutover,head_cipher_sha256}'IS DISTINCT FROM b.outer_cipher_sha256 THEN RAISE EXCEPTION 'checkpoint requires accepted baseline' USING ERRCODE='PST02';END IF;
  END IF;
 ELSE
  IF(to_jsonb(NEW)-'state'-'room_id'-'event_id'-'accepted_at')IS DISTINCT FROM(to_jsonb(OLD)-'state'-'room_id'-'event_id'-'accepted_at')
  OR NOT((OLD.state='prepared' AND NEW.state='sending')OR(OLD.state='sending' AND NEW.state IN('ambiguous','accepted'))OR(OLD.state='ambiguous'AND NEW.state='accepted'))
  OR(NEW.state='accepted'AND NEW.room_id IS DISTINCT FROM p.room_id)
  THEN RAISE EXCEPTION 'bootstrap send is immutable and must be reconciled' USING ERRCODE='PST02';END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER resource_e2e_bootstrap_slot_guard BEFORE INSERT OR UPDATE OR DELETE ON resource_e2e_bootstrap_slots FOR EACH ROW EXECUTE FUNCTION sytue_resource_e2e_bootstrap_slot_guard();

-- A pre-existing epoch-only caller cannot commit an active resource while a
-- pre-reference bootstrap is still preparing, even after both slots accepted.
CREATE FUNCTION sytue_resource_e2e_bootstrap_complete()RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM resource_e2e_bootstraps b WHERE(b.tenant,b.product,b.kind,b.resource_id)=(NEW.tenant,NEW.product,NEW.kind,NEW.resource_id)AND b.state<>'active')THEN RAISE EXCEPTION 'bootstrap activation must commit atomically' USING ERRCODE='PST02';END IF;
 RETURN NEW;
END $$;
CREATE CONSTRAINT TRIGGER resource_e2e_bootstrap_complete AFTER INSERT ON resource_e2e_resources DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION sytue_resource_e2e_bootstrap_complete();
