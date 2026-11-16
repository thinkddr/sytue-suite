-- SPDX-License-Identifier: Apache-2.0
-- Retain every original Calendar column; devices still have to verify the E2E
-- replacement. The shared implementation owns locking and state transitions.
-- sytue:private-storage-fence-v1
CREATE TABLE calendar_private_storage_states (
 tenant text NOT NULL,
 owner_sub text NOT NULL CHECK (owner_sub<>''),
 mode text NOT NULL DEFAULT 'legacy' CHECK (mode IN ('legacy','building','frozen','private')),
 epoch uuid,
 started_at timestamptz,
 source_rows bigint NOT NULL DEFAULT 0 CHECK (source_rows>=0),
 source_blobs bigint NOT NULL DEFAULT 0 CHECK (source_blobs=0),
 PRIMARY KEY(tenant,owner_sub),
 CHECK ((mode='legacy' AND epoch IS NULL AND started_at IS NULL)
     OR (mode<>'legacy' AND epoch IS NOT NULL AND started_at IS NOT NULL))
);
ALTER TABLE calendar_private_storage_states ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_private_storage_states FORCE ROW LEVEL SECURITY;
CREATE POLICY calendar_private_state_owner ON calendar_private_storage_states
 USING (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true))
 WITH CHECK (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY calendar_private_state_guard ON calendar_private_storage_states
 USING (current_setting('app.maintenance',true)='on')
 WITH CHECK (current_setting('app.maintenance',true)='on');
CREATE POLICY calendar_private_state_merge ON calendar_private_storage_states
 USING (current_setting('app.merge',true)='on') WITH CHECK (current_setting('app.merge',true)='on');
CREATE TRIGGER calendar_private_state_scope_guard BEFORE UPDATE OR DELETE ON calendar_private_storage_states
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_state_scope_guard();
CREATE CONSTRAINT TRIGGER calendar_private_capture_completed
 AFTER INSERT OR UPDATE ON calendar_private_storage_states DEFERRABLE INITIALLY DEFERRED
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_build_completed();

CREATE TABLE calendar_private_storage_snapshot (
 tenant text NOT NULL,
 owner_sub text NOT NULL,
 epoch uuid NOT NULL,
 kind text COLLATE "C" NOT NULL CHECK (kind IN ('calendar','object','event','attendee','invitation','reminder','subscription','share','request','resource','outbox')),
 id text COLLATE "C" NOT NULL,
 payload bytea NOT NULL CHECK (octet_length(payload) BETWEEN 2 AND 8388608),
 size_bytes bigint GENERATED ALWAYS AS (octet_length(payload)) STORED,
 sha256 text GENERATED ALWAYS AS (encode(digest(payload,'sha256'),'hex')) STORED,
 PRIMARY KEY(tenant,owner_sub,epoch,kind,id),
 FOREIGN KEY(tenant,owner_sub) REFERENCES calendar_private_storage_states(tenant,owner_sub)
);
ALTER TABLE calendar_private_storage_snapshot ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_private_storage_snapshot FORCE ROW LEVEL SECURITY;
CREATE POLICY calendar_private_snapshot_read ON calendar_private_storage_snapshot FOR SELECT
 USING (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY calendar_private_snapshot_capture ON calendar_private_storage_snapshot FOR INSERT
 WITH CHECK (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true)
 AND EXISTS(SELECT 1 FROM calendar_private_storage_states s WHERE s.tenant=calendar_private_storage_snapshot.tenant
 AND s.owner_sub=calendar_private_storage_snapshot.owner_sub AND s.epoch=calendar_private_storage_snapshot.epoch AND s.mode='building'));
CREATE POLICY calendar_private_snapshot_merge ON calendar_private_storage_snapshot
 USING (current_setting('app.merge',true)='on') WITH CHECK (current_setting('app.merge',true)='on');
CREATE TRIGGER calendar_private_snapshot_immutable BEFORE UPDATE OR DELETE ON calendar_private_storage_snapshot
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_snapshot_immutable();

-- Only routing identities verified by Sytue ID belong here. Historical messages
-- never acquire an owner merely because an address now appears in a token.
CREATE TABLE calendar_private_storage_aliases (
 tenant text NOT NULL,
 owner_sub text NOT NULL,
 epoch uuid NOT NULL,
 address text COLLATE "C" NOT NULL CHECK (address<>'' AND address=lower(btrim(address)) AND octet_length(address)<=1024),
 PRIMARY KEY(tenant,owner_sub,address),
 FOREIGN KEY(tenant,owner_sub) REFERENCES calendar_private_storage_states(tenant,owner_sub)
);
CREATE INDEX calendar_private_alias_address ON calendar_private_storage_aliases(address);
ALTER TABLE calendar_private_storage_aliases ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_private_storage_aliases FORCE ROW LEVEL SECURITY;
CREATE POLICY calendar_private_alias_read ON calendar_private_storage_aliases FOR SELECT
 USING ((tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true)) OR current_setting('app.maintenance',true)='on');
CREATE POLICY calendar_private_alias_capture ON calendar_private_storage_aliases FOR INSERT
 WITH CHECK (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true)
 AND EXISTS(SELECT 1 FROM calendar_private_storage_states s WHERE s.tenant=calendar_private_storage_aliases.tenant
 AND s.owner_sub=calendar_private_storage_aliases.owner_sub AND s.epoch=calendar_private_storage_aliases.epoch AND s.mode='building'));
CREATE POLICY calendar_private_alias_merge ON calendar_private_storage_aliases
 USING (current_setting('app.merge',true)='on') WITH CHECK (current_setting('app.merge',true)='on');
CREATE TRIGGER calendar_private_alias_immutable BEFORE UPDATE OR DELETE ON calendar_private_storage_aliases
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_snapshot_immutable();

-- Calendar ingress needs identity locks in addition to the shared owner-row
-- fence: a writer can arrive from another tenant before any alias exists. The
-- exclusive side runs before BeginPrivateStorageSnapshot; all legacy writers
-- take the shared side before resolving the latest aliases. RC gives the SQL
-- inside these volatile functions a fresh snapshot after waiting; RR/serializable
-- ingress fails closed instead of consulting a stale, missing alias.
CREATE FUNCTION calendar_private_lock_identities(subject text,addresses text[]) RETURNS void LANGUAGE plpgsql
 SET search_path=pg_catalog,public AS $$
DECLARE identity text;
BEGIN
 IF current_setting('transaction_isolation')<>'read committed' THEN
  RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='calendar_private_identity_requires_read_committed';
 END IF;
 IF subject IS NULL OR subject='' OR octet_length(subject)>1024 THEN
  RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='calendar_private_identity_invalid';
 END IF;
 FOR identity IN SELECT DISTINCT v FROM (
  SELECT 's:'||subject AS v UNION ALL SELECT 'e:'||lower(btrim(a)) FROM unnest(addresses) x(a) WHERE a<>''
 ) x ORDER BY v
 LOOP
  PERFORM pg_advisory_xact_lock(hashtextextended('calendar:private:'||identity,0));
 END LOOP;
END;
$$;
CREATE FUNCTION calendar_private_capture_insert_guard() RETURNS trigger LANGUAGE plpgsql
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.calendar_private_storage_states s WHERE s.tenant=NEW.tenant
  AND s.owner_sub=NEW.owner_sub AND s.epoch=NEW.epoch AND s.mode='building') THEN
  RAISE EXCEPTION USING ERRCODE='PST01',MESSAGE='private_storage_source_immutable';
 END IF;
 IF TG_TABLE_NAME='calendar_private_storage_aliases' THEN
  PERFORM public.calendar_private_lock_identities(NEW.owner_sub,ARRAY[NEW.address]);
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER calendar_private_snapshot_insert_guard BEFORE INSERT ON calendar_private_storage_snapshot
 FOR EACH ROW EXECUTE FUNCTION calendar_private_capture_insert_guard();
CREATE TRIGGER calendar_private_alias_insert_guard BEFORE INSERT ON calendar_private_storage_aliases
 FOR EACH ROW EXECUTE FUNCTION calendar_private_capture_insert_guard();
CREATE FUNCTION calendar_private_lock_owner(subject text) RETURNS void LANGUAGE plpgsql
 SET search_path=pg_catalog,public AS $$
BEGIN
 IF coalesce(subject,'')='' OR octet_length(subject)>1024 THEN
  RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='calendar_private_identity_invalid';
 END IF;
 PERFORM pg_advisory_xact_lock_shared(hashtextextended('calendar:private:s:'||subject,0));
END;
$$;
CREATE FUNCTION calendar_private_identity_guard(p_subject text,p_address text) RETURNS void LANGUAGE plpgsql
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE identity text; candidate record;
BEGIN
 IF coalesce(p_subject,'')='' AND coalesce(btrim(p_address),'')='' THEN RETURN; END IF;
 IF current_setting('transaction_isolation')<>'read committed' THEN
  RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='calendar_private_identity_requires_read_committed';
 END IF;
 FOR identity IN SELECT DISTINCT v FROM (
  SELECT 's:'||p_subject AS v WHERE coalesce(p_subject,'')<>''
  UNION ALL SELECT 'e:'||lower(btrim(p_address)) WHERE coalesce(btrim(p_address),'')<>''
 ) x ORDER BY v
 LOOP
  PERFORM pg_advisory_xact_lock_shared(hashtextextended('calendar:private:'||identity,0));
 END LOOP;
 FOR candidate IN
  SELECT s.tenant,s.owner_sub FROM public.calendar_private_storage_states s WHERE s.owner_sub=p_subject
  UNION SELECT a.tenant,a.owner_sub FROM public.calendar_private_storage_aliases a WHERE a.address=lower(btrim(p_address))
  ORDER BY 1,2
 LOOP
  PERFORM public.sytue_private_storage_guard('public.calendar_private_storage_states'::regclass,candidate.tenant,candidate.owner_sub);
 END LOOP;
END;
$$;

-- Workers route their rows through calendars/events without gaining maintenance
-- writes to those parents. Tenant business requests keep their existing RLS.
CREATE POLICY calendars_private_routing ON calendars FOR SELECT USING (current_setting('app.maintenance',true)='on');
CREATE POLICY events_private_routing ON events FOR SELECT USING (current_setting('app.maintenance',true)='on');
CREATE POLICY calendar_objects_private_routing ON calendar_objects FOR SELECT USING (current_setting('app.maintenance',true)='on');
CREATE POLICY calendar_resources_private_routing ON calendar_resources FOR SELECT USING (current_setting('app.maintenance',true)='on');

CREATE FUNCTION calendar_private_calendar_frozen(t text,c uuid) RETURNS boolean LANGUAGE sql STABLE
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
 SELECT EXISTS(SELECT 1 FROM public.calendars b WHERE b.tenant=t AND b.id=c
  AND public.sytue_private_storage_frozen('public.calendar_private_storage_states'::regclass,b.tenant,b.owner_sub));
$$;

-- Routing only: OLD and NEW both participate, including moving a child between
-- calendars, synthetic resource calendars and incoming/outgoing relationships.
CREATE FUNCTION calendar_private_row_guard() RETURNS trigger LANGUAGE plpgsql
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE value jsonb; parent_calendar uuid; parent_event uuid; candidate record; subject text;
BEGIN
 FOR value IN SELECT v FROM (VALUES
  (CASE WHEN TG_OP<>'INSERT' THEN to_jsonb(OLD) END),
  (CASE WHEN TG_OP<>'DELETE' THEN to_jsonb(NEW) END)) x(v) WHERE v IS NOT NULL
 LOOP
  parent_calendar:=NULL; parent_event:=NULL;
  IF TG_TABLE_NAME='calendars' THEN
   parent_calendar:=(value->>'id')::uuid;
   PERFORM public.calendar_private_lock_owner(value->>'owner_sub');
   PERFORM public.sytue_private_storage_guard('public.calendar_private_storage_states'::regclass,value->>'tenant',value->>'owner_sub');
  ELSIF TG_TABLE_NAME IN ('calendar_objects','events','calendar_reminders','calendar_subscriptions','calendar_shares','calendar_peticiones_edicion','calendar_resources') THEN
   parent_calendar:=(value->>'calendar_id')::uuid;
  ELSIF TG_TABLE_NAME IN ('attendees','invitations') THEN
   parent_event:=(value->>'event_id')::uuid;
  ELSIF TG_TABLE_NAME='events_outbox' THEN
   FOR candidate IN
    SELECT c.tenant,c.owner_sub FROM public.calendars c
     WHERE c.tenant=value->>'tenant' AND c.id::text=value->'payload'->>'calendar_id'
    UNION SELECT c.tenant,c.owner_sub FROM public.events e JOIN public.calendars c ON c.tenant=e.tenant AND c.id=e.calendar_id
     WHERE e.tenant=value->>'tenant' AND e.id::text=value->'payload'->>'event_id'
    UNION SELECT c.tenant,c.owner_sub FROM public.invitations i JOIN public.events e ON e.tenant=i.tenant AND e.id=i.event_id
     JOIN public.calendars c ON c.tenant=e.tenant AND c.id=e.calendar_id
     WHERE i.tenant=value->>'tenant' AND i.id::text=value->'payload'->>'invitation_id'
    UNION SELECT r.tenant,r.created_by FROM public.calendar_resources r
     WHERE r.tenant=value->>'tenant' AND r.id::text=value->'payload'->>'resource_id' AND r.created_by<>''
    UNION SELECT i.tenant,i.recipient_sub FROM public.invitations i
     WHERE i.tenant=value->>'tenant' AND i.id::text=value->'payload'->>'invitation_id' AND i.recipient_sub<>''
    UNION SELECT r.tenant,m.subject FROM public.calendar_resources r CROSS JOIN LATERAL unnest(r.managers) m(subject)
     WHERE r.tenant=value->>'tenant' AND r.id::text=value->'payload'->>'resource_id' AND m.subject<>''
    ORDER BY 1,2
   LOOP
    PERFORM public.sytue_private_storage_guard('public.calendar_private_storage_states'::regclass,candidate.tenant,candidate.owner_sub);
   END LOOP;
   subject:=value->'payload'->>'owner_sub';
   IF coalesce(subject,'')<>'' THEN
    PERFORM public.calendar_private_lock_owner(subject);
    PERFORM public.sytue_private_storage_guard('public.calendar_private_storage_states'::regclass,value->>'tenant',subject);
   END IF;
  END IF;
  IF parent_event IS NOT NULL THEN
   SELECT e.calendar_id INTO parent_calendar FROM public.events e WHERE e.tenant=value->>'tenant' AND e.id=parent_event;
  END IF;
  FOR candidate IN SELECT c.tenant,c.owner_sub FROM public.calendars c
   WHERE c.tenant=value->>'tenant' AND c.id=parent_calendar ORDER BY c.tenant,c.owner_sub
  LOOP
   PERFORM public.calendar_private_lock_owner(candidate.owner_sub);
   PERFORM public.sytue_private_storage_guard('public.calendar_private_storage_states'::regclass,candidate.tenant,candidate.owner_sub);
  END LOOP;
  FOR candidate IN
   SELECT r.tenant,r.created_by AS owner_sub FROM public.calendar_resources r
    WHERE r.tenant=value->>'tenant' AND r.calendar_id=parent_calendar AND r.created_by<>''
   UNION SELECT r.tenant,m.subject FROM public.calendar_resources r CROSS JOIN LATERAL unnest(r.managers) m(subject)
    WHERE r.tenant=value->>'tenant' AND r.calendar_id=parent_calendar AND m.subject<>'' ORDER BY 1,2
  LOOP
   PERFORM public.calendar_private_identity_guard(candidate.owner_sub,NULL);
   PERFORM public.sytue_private_storage_guard('public.calendar_private_storage_states'::regclass,candidate.tenant,candidate.owner_sub);
  END LOOP;
  FOR subject IN SELECT DISTINCT s FROM (VALUES
   (CASE WHEN TG_TABLE_NAME='calendar_reminders' THEN value->>'owner_sub' END),
   (CASE WHEN TG_TABLE_NAME='invitations' THEN value->>'recipient_sub' END),
   (CASE WHEN TG_TABLE_NAME='calendar_shares' THEN value->>'grantee_sub' END),
   (CASE WHEN TG_TABLE_NAME='calendar_peticiones_edicion' THEN value->>'sujeto' END),
   (CASE WHEN TG_TABLE_NAME='calendar_resources' THEN value->>'created_by' END)) x(s) WHERE s IS NOT NULL AND s<>'' ORDER BY s
  LOOP
   PERFORM public.calendar_private_lock_owner(subject);
   PERFORM public.sytue_private_storage_guard('public.calendar_private_storage_states'::regclass,value->>'tenant',subject);
  END LOOP;
  IF TG_TABLE_NAME='invitations' THEN
   PERFORM public.calendar_private_identity_guard(value->>'recipient_sub',value->>'recipient');
  ELSIF TG_TABLE_NAME='calendar_shares' THEN
   PERFORM public.calendar_private_identity_guard(value->>'grantee_sub',value->>'grantee_email');
  ELSIF TG_TABLE_NAME='attendees' THEN
   PERFORM public.calendar_private_identity_guard(NULL,value->>'email');
  END IF;
  IF TG_TABLE_NAME='calendar_resources' THEN
   FOR subject IN SELECT DISTINCT s FROM jsonb_array_elements_text(value->'managers') x(s) WHERE s<>'' ORDER BY s
   LOOP
    PERFORM public.calendar_private_identity_guard(subject,NULL);
    PERFORM public.sytue_private_storage_guard('public.calendar_private_storage_states'::regclass,value->>'tenant',subject);
   END LOOP;
  END IF;
 END LOOP;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER calendar_private_calendars_guard BEFORE INSERT OR UPDATE OR DELETE ON calendars FOR EACH ROW EXECUTE FUNCTION calendar_private_row_guard();
CREATE TRIGGER calendar_private_objects_guard BEFORE INSERT OR UPDATE OR DELETE ON calendar_objects FOR EACH ROW EXECUTE FUNCTION calendar_private_row_guard();
CREATE TRIGGER calendar_private_events_guard BEFORE INSERT OR UPDATE OR DELETE ON events FOR EACH ROW EXECUTE FUNCTION calendar_private_row_guard();
CREATE TRIGGER calendar_private_attendees_guard BEFORE INSERT OR UPDATE OR DELETE ON attendees FOR EACH ROW EXECUTE FUNCTION calendar_private_row_guard();
CREATE TRIGGER calendar_private_invitations_guard BEFORE INSERT OR UPDATE OR DELETE ON invitations FOR EACH ROW EXECUTE FUNCTION calendar_private_row_guard();
CREATE TRIGGER calendar_private_reminders_guard BEFORE INSERT OR UPDATE OR DELETE ON calendar_reminders FOR EACH ROW EXECUTE FUNCTION calendar_private_row_guard();
CREATE TRIGGER calendar_private_subscriptions_guard BEFORE INSERT OR UPDATE OR DELETE ON calendar_subscriptions FOR EACH ROW EXECUTE FUNCTION calendar_private_row_guard();
CREATE TRIGGER calendar_private_shares_guard BEFORE INSERT OR UPDATE OR DELETE ON calendar_shares FOR EACH ROW EXECUTE FUNCTION calendar_private_row_guard();
CREATE TRIGGER calendar_private_requests_guard BEFORE INSERT OR UPDATE OR DELETE ON calendar_peticiones_edicion FOR EACH ROW EXECUTE FUNCTION calendar_private_row_guard();
CREATE TRIGGER calendar_private_resources_guard BEFORE INSERT OR UPDATE OR DELETE ON calendar_resources FOR EACH ROW EXECUTE FUNCTION calendar_private_row_guard();
CREATE TRIGGER calendar_private_outbox_guard BEFORE INSERT OR UPDATE OR DELETE ON events_outbox FOR EACH ROW EXECUTE FUNCTION calendar_private_row_guard();
