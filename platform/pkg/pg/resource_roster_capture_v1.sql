-- SPDX-License-Identifier: Apache-2.0
-- PUBLIC pre-verification capture belongs to the SAME relay/source. No key,
-- grant, epoch, accepted message or routing permit is stored in this phase.
CREATE TABLE resource_e2e_roster_captures(
 tenant text NOT NULL,product text NOT NULL,kind text NOT NULL,resource_id text NOT NULL,
 plan_id uuid NOT NULL UNIQUE,owner_sub text NOT NULL,room_id text NOT NULL,
 source_epoch uuid NOT NULL,source_seq bigint NOT NULL CHECK(source_seq>=0),
 acl_version bigint NOT NULL CHECK(acl_version>=0),session_generation bigint NOT NULL CHECK(session_generation>0),
 creator_user text NOT NULL,creator_device text NOT NULL,creator_root text NOT NULL,
 session_binding_sha256 text NOT NULL CHECK(session_binding_sha256~'^[a-f0-9]{64}$'),
 plan bytea NOT NULL,plan_sha256 text NOT NULL CHECK(plan_sha256=encode(digest(plan,'sha256'),'hex')),
 signed_plan bytea NOT NULL,manifest bytea NOT NULL,
 manifest_sha256 text NOT NULL CHECK(manifest_sha256=encode(digest(manifest,'sha256'),'hex')),
 member_count bigint NOT NULL CHECK(member_count>0),stream_sha256 text NOT NULL CHECK(stream_sha256~'^[a-f0-9]{64}$'),
 received_count bigint NOT NULL DEFAULT 0 CHECK(received_count>=0 AND received_count<=member_count),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN('pending','cancelled','complete')),
 created_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz,
 PRIMARY KEY(tenant,product,kind,resource_id,plan_id),
 CHECK((state='pending' AND finished_at IS NULL)OR(state IN('cancelled','complete')AND finished_at IS NOT NULL)),
 CHECK((convert_from(signed_plan,'UTF8')::jsonb)-'signatures'=convert_from(plan,'UTF8')::jsonb),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,resource,tenant}'=tenant,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,resource,product}'=product,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,resource,kind}'=kind,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,resource,id}'=resource_id,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)->>'id'=plan_id::text,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)->>'room_id'=room_id,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,owner_scope,tenant}'=tenant,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,owner_scope,product}'=product,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,owner_scope,subject}'=owner_sub,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,source_epoch}'=source_epoch::text,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,source_seq}'=source_seq::text,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{source,acl_version}'=acl_version::text,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{creator,user_id}'=creator_user,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{creator,device_id}'=creator_device,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)#>>'{creator,root_key}'=creator_root,false)),
 CHECK(coalesce((convert_from(plan,'UTF8')::jsonb)->>'session_binding_sha256'=session_binding_sha256,false)),
 CHECK(coalesce((convert_from(manifest,'UTF8')::jsonb)->>'v'='1',false)),
 CHECK(coalesce((convert_from(manifest,'UTF8')::jsonb)->>'domain'='org.sytue.resource.bootstrap.roster-manifest.v1',false)),
 CHECK(coalesce((convert_from(manifest,'UTF8')::jsonb)->>'plan_sha256'=plan_sha256,false)),
 CHECK(coalesce((convert_from(manifest,'UTF8')::jsonb)->>'member_count'=member_count::text,false)),
 CHECK(coalesce((convert_from(manifest,'UTF8')::jsonb)->>'ordered_inputs_sha256'=stream_sha256,false))
);
CREATE UNIQUE INDEX resource_e2e_roster_current ON resource_e2e_roster_captures(tenant,product,kind,resource_id)WHERE state<>'cancelled';
CREATE TABLE resource_e2e_roster_pages(
 tenant text NOT NULL,product text NOT NULL,kind text NOT NULL,resource_id text NOT NULL,plan_id uuid NOT NULL,
 input_offset bigint NOT NULL CHECK(input_offset>=0),input_count bigint NOT NULL CHECK(input_count>0),
 value bytea NOT NULL CHECK(octet_length(value)<=131072),
 sha256 text NOT NULL CHECK(sha256=encode(digest(value,'sha256'),'hex')),
 first_key bytea NOT NULL,last_key bytea NOT NULL,CHECK(first_key<=last_key),
 PRIMARY KEY(tenant,product,kind,resource_id,plan_id,input_offset),
 FOREIGN KEY(tenant,product,kind,resource_id,plan_id)REFERENCES resource_e2e_roster_captures(tenant,product,kind,resource_id,plan_id),
 CHECK(coalesce((convert_from(value,'UTF8')::jsonb)->>'offset'=input_offset::text,false)),
 CHECK(coalesce(jsonb_array_length((convert_from(value,'UTF8')::jsonb)->'inputs')=input_count,false))
);
DO $$ DECLARE relation text;BEGIN
 FOREACH relation IN ARRAY ARRAY['resource_e2e_roster_captures','resource_e2e_roster_pages'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',relation);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',relation);
  EXECUTE format($policy$CREATE POLICY resource_e2e_tenant ON %I USING(tenant=current_setting('app.tenant',true))WITH CHECK(tenant=current_setting('app.tenant',true))$policy$,relation);
  EXECUTE format('CREATE TRIGGER resource_e2e_no_truncate BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION sytue_resource_e2e_immutable()',relation);
 END LOOP;
END $$;
CREATE FUNCTION sytue_resource_e2e_roster_capture_guard()RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE actual bigint;bootstrap_row record;
BEGIN
 IF TG_OP='DELETE'THEN RAISE EXCEPTION 'public capture evidence retained' USING ERRCODE='PST02';END IF;
 IF TG_OP='INSERT'THEN
  IF NEW.state<>'pending'OR NEW.received_count<>0 OR EXISTS(SELECT 1 FROM resource_e2e_bootstraps b WHERE(b.tenant,b.product,b.kind,b.resource_id)=(NEW.tenant,NEW.product,NEW.kind,NEW.resource_id))OR EXISTS(SELECT 1 FROM resource_e2e_resources r WHERE(r.tenant,r.product,r.kind,r.resource_id)=(NEW.tenant,NEW.product,NEW.kind,NEW.resource_id))THEN RAISE EXCEPTION 'capture starts before bootstrap only' USING ERRCODE='PST02';END IF;
  RETURN NEW;
 END IF;
 IF(to_jsonb(OLD)-'state'-'received_count'-'finished_at')IS DISTINCT FROM(to_jsonb(NEW)-'state'-'received_count'-'finished_at')OR OLD.state<>'pending'THEN RAISE EXCEPTION 'capture manifest immutable' USING ERRCODE='PST02';END IF;
 SELECT coalesce(sum(input_count),0)INTO actual FROM resource_e2e_roster_pages p WHERE(p.tenant,p.product,p.kind,p.resource_id,p.plan_id)=(NEW.tenant,NEW.product,NEW.kind,NEW.resource_id,NEW.plan_id);
 IF NEW.received_count<>actual OR NEW.received_count<OLD.received_count THEN RAISE EXCEPTION 'capture progress requires exact immutable pages' USING ERRCODE='PST02';END IF;
 IF NEW.state='complete'THEN
  SELECT * INTO bootstrap_row FROM resource_e2e_bootstraps p WHERE(p.tenant,p.product,p.kind,p.resource_id,p.plan_id)=(NEW.tenant,NEW.product,NEW.kind,NEW.resource_id,NEW.plan_id);
  IF NEW.received_count<>NEW.member_count OR bootstrap_row.plan_sha256 IS DISTINCT FROM NEW.plan_sha256 OR bootstrap_row.signed_plan IS DISTINCT FROM NEW.signed_plan THEN RAISE EXCEPTION 'capture requires complete shared verified plan' USING ERRCODE='PST02';END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER resource_e2e_roster_capture_guard BEFORE INSERT OR UPDATE OR DELETE ON resource_e2e_roster_captures FOR EACH ROW EXECUTE FUNCTION sytue_resource_e2e_roster_capture_guard();
CREATE FUNCTION sytue_resource_e2e_roster_page_guard()RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p record;j jsonb;item jsonb;current_key bytea;previous_key bytea;first_key bytea;last_key bytea;previous_last bytea;
BEGIN
 IF TG_OP<>'INSERT'THEN RAISE EXCEPTION 'capture page immutable' USING ERRCODE='PST02';END IF;
 SELECT * INTO p FROM resource_e2e_roster_captures c WHERE(c.tenant,c.product,c.kind,c.resource_id,c.plan_id)=(NEW.tenant,NEW.product,NEW.kind,NEW.resource_id,NEW.plan_id)FOR UPDATE;
 IF p.plan_id IS NULL OR p.state<>'pending'OR NEW.input_offset<>p.received_count OR NEW.input_count>p.member_count-p.received_count THEN RAISE EXCEPTION 'capture page must append exact range' USING ERRCODE='PST02';END IF;
 j:=convert_from(NEW.value,'UTF8')::jsonb;
 FOR item IN SELECT value FROM jsonb_array_elements(j->'inputs')LOOP
  current_key:=convert_to(item#>>'{member,scope,subject}','UTF8')||decode('00','hex')||convert_to(item#>>'{member,user_id}','UTF8')||decode('00','hex')||convert_to(item#>>'{member,product_key_id}','UTF8');
  IF current_key IS NULL OR(previous_key IS NOT NULL AND previous_key>=current_key)OR item#>>'{member,scope,tenant}'IS DISTINCT FROM p.tenant OR item#>>'{member,scope,product}'IS DISTINCT FROM p.product THEN RAISE EXCEPTION 'capture page noncanonical member order' USING ERRCODE='PST02';END IF;
  IF first_key IS NULL THEN first_key:=current_key;END IF;last_key:=current_key;previous_key:=current_key;
 END LOOP;
 SELECT s.last_key INTO previous_last FROM resource_e2e_roster_pages s WHERE(s.tenant,s.product,s.kind,s.resource_id,s.plan_id)=(NEW.tenant,NEW.product,NEW.kind,NEW.resource_id,NEW.plan_id)ORDER BY input_offset DESC LIMIT 1;
 IF first_key IS DISTINCT FROM NEW.first_key OR last_key IS DISTINCT FROM NEW.last_key OR(previous_last IS NOT NULL AND previous_last>=first_key)THEN RAISE EXCEPTION 'capture global member order mismatch' USING ERRCODE='PST02';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER resource_e2e_roster_page_guard BEFORE INSERT OR UPDATE OR DELETE ON resource_e2e_roster_pages FOR EACH ROW EXECUTE FUNCTION sytue_resource_e2e_roster_page_guard();
CREATE FUNCTION sytue_resource_e2e_roster_page_progress()RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE resource_e2e_roster_captures SET received_count=received_count+NEW.input_count WHERE(tenant,product,kind,resource_id,plan_id)=(NEW.tenant,NEW.product,NEW.kind,NEW.resource_id,NEW.plan_id);
 RETURN NEW;
END $$;
CREATE TRIGGER resource_e2e_roster_page_progress AFTER INSERT ON resource_e2e_roster_pages FOR EACH ROW EXECUTE FUNCTION sytue_resource_e2e_roster_page_progress();
