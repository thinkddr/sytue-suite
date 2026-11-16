-- SPDX-License-Identifier: Apache-2.0
-- Retained migration input, including deleted rows. This is not an encrypted
-- store: verified devices must publish and verify its E2E replacement first.
-- sytue:private-storage-fence-v1

CREATE TABLE contacts_private_storage_states (
 tenant text NOT NULL,
 owner_sub text NOT NULL CHECK (owner_sub<>''),
 mode text NOT NULL DEFAULT 'legacy' CHECK (mode IN ('legacy','building','frozen','private')),
 epoch uuid,
 started_at timestamptz,
 source_rows bigint NOT NULL DEFAULT 0 CHECK (source_rows>=0),
 source_blobs bigint NOT NULL DEFAULT 0 CHECK (source_blobs>=0 AND source_blobs<=source_rows),
 PRIMARY KEY(tenant,owner_sub),
 CHECK ((mode='legacy' AND epoch IS NULL AND started_at IS NULL)
     OR (mode<>'legacy' AND epoch IS NOT NULL AND started_at IS NOT NULL))
);
ALTER TABLE contacts_private_storage_states ENABLE ROW LEVEL SECURITY;
ALTER TABLE contacts_private_storage_states FORCE ROW LEVEL SECURITY;
CREATE POLICY contacts_private_state_owner ON contacts_private_storage_states
 USING (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true))
 WITH CHECK (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY contacts_private_state_guard ON contacts_private_storage_states
 USING (current_setting('app.maintenance',true)='on')
 WITH CHECK (current_setting('app.maintenance',true)='on');

CREATE TABLE contacts_private_storage_snapshot (
 tenant text NOT NULL,
 owner_sub text NOT NULL,
 epoch uuid NOT NULL,
 kind text COLLATE "C" NOT NULL CHECK (kind IN ('book','card','contact','email','phone','group','member')),
 id text COLLATE "C" NOT NULL,
 payload bytea NOT NULL CHECK (octet_length(payload) BETWEEN 2 AND 8388608),
 sha256 text GENERATED ALWAYS AS (encode(digest(payload,'sha256'),'hex')) STORED,
 PRIMARY KEY(tenant,owner_sub,epoch,kind,id),
 FOREIGN KEY(tenant,owner_sub) REFERENCES contacts_private_storage_states(tenant,owner_sub)
);
ALTER TABLE contacts_private_storage_snapshot ENABLE ROW LEVEL SECURITY;
ALTER TABLE contacts_private_storage_snapshot FORCE ROW LEVEL SECURITY;
CREATE POLICY contacts_private_snapshot_read ON contacts_private_storage_snapshot FOR SELECT
 USING (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY contacts_private_snapshot_capture ON contacts_private_storage_snapshot FOR INSERT
 WITH CHECK (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true)
 AND EXISTS(SELECT 1 FROM contacts_private_storage_states s WHERE s.tenant=contacts_private_storage_snapshot.tenant
 AND s.owner_sub=contacts_private_storage_snapshot.owner_sub AND s.epoch=contacts_private_storage_snapshot.epoch AND s.mode='building'));
-- No UPDATE/DELETE policies: neither a caller acknowledgement nor another
-- ordinary metadata operation is sufficient evidence to retire plaintext.
CREATE CONSTRAINT TRIGGER contacts_private_capture_completed
 AFTER INSERT OR UPDATE ON contacts_private_storage_states DEFERRABLE INITIALLY DEFERRED
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_build_completed();

-- Domain-specific resolution only. Locking, RR conflict detection and the
-- deny-after-freeze rule are owned by platform/pkg/pg's shared SQL function.
CREATE FUNCTION contacts_private_row_guard() RETURNS trigger LANGUAGE plpgsql
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE value jsonb; parent_book uuid; candidate record;
BEGIN
 FOR value IN SELECT v FROM (VALUES
   (CASE WHEN TG_OP<>'INSERT' THEN to_jsonb(OLD) END),
   (CASE WHEN TG_OP<>'DELETE' THEN to_jsonb(NEW) END)) x(v) WHERE v IS NOT NULL
 LOOP
  IF TG_TABLE_NAME='address_books' THEN
   PERFORM public.sytue_private_storage_guard('public.contacts_private_storage_states'::regclass,value->>'tenant',value->>'owner_sub');
  ELSE
   parent_book := NULL;
   IF TG_TABLE_NAME IN ('card_objects','contacts','contact_groups') THEN
    parent_book := (value->>'address_book_id')::uuid;
   ELSIF TG_TABLE_NAME IN ('contact_emails','contact_phones') THEN
    SELECT c.address_book_id INTO parent_book FROM public.contacts c
      WHERE c.tenant=value->>'tenant' AND c.id=(value->>'contact_id')::uuid;
   ELSIF TG_TABLE_NAME='contact_group_members' THEN
    SELECT g.address_book_id INTO parent_book FROM public.contact_groups g
      WHERE g.tenant=value->>'tenant' AND g.id=(value->>'group_id')::uuid;
   END IF;
   FOR candidate IN SELECT b.tenant,b.owner_sub FROM public.address_books b
     WHERE b.tenant=value->>'tenant' AND b.id=parent_book ORDER BY b.tenant,b.owner_sub
   LOOP
    PERFORM public.sytue_private_storage_guard('public.contacts_private_storage_states'::regclass,candidate.tenant,candidate.owner_sub);
   END LOOP;
   -- A resolved MEMBER may reference another local contact. Guard its owner as
   -- well: linking an unfrozen group must not mutate a frozen owner's graph.
   IF TG_TABLE_NAME='contact_group_members' AND value->>'contact_id' IS NOT NULL THEN
    FOR candidate IN SELECT b.tenant,b.owner_sub FROM public.contacts c
      JOIN public.address_books b ON b.tenant=c.tenant AND b.id=c.address_book_id
      WHERE c.tenant=value->>'tenant' AND c.id=(value->>'contact_id')::uuid ORDER BY b.tenant,b.owner_sub
    LOOP
     PERFORM public.sytue_private_storage_guard('public.contacts_private_storage_states'::regclass,candidate.tenant,candidate.owner_sub);
    END LOOP;
   END IF;
  END IF;
 END LOOP;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$;

CREATE TRIGGER contacts_private_books_guard BEFORE INSERT OR UPDATE OR DELETE ON address_books FOR EACH ROW EXECUTE FUNCTION contacts_private_row_guard();
CREATE TRIGGER contacts_private_cards_guard BEFORE INSERT OR UPDATE OR DELETE ON card_objects FOR EACH ROW EXECUTE FUNCTION contacts_private_row_guard();
CREATE TRIGGER contacts_private_people_guard BEFORE INSERT OR UPDATE OR DELETE ON contacts FOR EACH ROW EXECUTE FUNCTION contacts_private_row_guard();
CREATE TRIGGER contacts_private_emails_guard BEFORE INSERT OR UPDATE OR DELETE ON contact_emails FOR EACH ROW EXECUTE FUNCTION contacts_private_row_guard();
CREATE TRIGGER contacts_private_phones_guard BEFORE INSERT OR UPDATE OR DELETE ON contact_phones FOR EACH ROW EXECUTE FUNCTION contacts_private_row_guard();
CREATE TRIGGER contacts_private_groups_guard BEFORE INSERT OR UPDATE OR DELETE ON contact_groups FOR EACH ROW EXECUTE FUNCTION contacts_private_row_guard();
CREATE TRIGGER contacts_private_members_guard BEFORE INSERT OR UPDATE OR DELETE ON contact_group_members FOR EACH ROW EXECUTE FUNCTION contacts_private_row_guard();
