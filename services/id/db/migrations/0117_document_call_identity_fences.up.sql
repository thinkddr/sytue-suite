-- SPDX-License-Identifier: Apache-2.0
-- ID scopes by subject + organization explicitly; no transport keys are stored.
CREATE SEQUENCE identity_call_access_version_seq AS bigint;
CREATE TABLE identity_call_guard_state(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),enabled boolean NOT NULL DEFAULT false);
INSERT INTO identity_call_guard_state(singleton) VALUES(true);
CREATE TABLE identity_call_access_fences(subject uuid NOT NULL,org_slug text NOT NULL,version bigint NOT NULL,acknowledged_version bigint NOT NULL DEFAULT 0,transaction_id xid8 NOT NULL,PRIMARY KEY(subject,org_slug));
CREATE INDEX identity_call_access_changed ON identity_call_access_fences(transaction_id);
ALTER TABLE identity_call_guard_state ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity_call_guard_state FORCE ROW LEVEL SECURITY;
CREATE POLICY identity_call_guard_read ON identity_call_guard_state FOR SELECT USING(true);
CREATE POLICY identity_call_guard_configure ON identity_call_guard_state FOR UPDATE USING(current_setting('app.maintenance',true)='on') WITH CHECK(current_setting('app.maintenance',true)='on');
CREATE FUNCTION id_record_call_identity(actor uuid,org text) RETURNS void LANGUAGE sql AS $$
 INSERT INTO identity_call_access_fences(subject,org_slug,version,transaction_id)
 SELECT actor,org,nextval('identity_call_access_version_seq'),pg_current_xact_id() WHERE org IS NOT NULL AND org<>''
 ON CONFLICT(subject,org_slug) DO UPDATE SET version=EXCLUDED.version,transaction_id=EXCLUDED.transaction_id;
$$;
CREATE FUNCTION id_touch_call_identity() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE actor uuid; org uuid; slug text; oldorg uuid;
BEGIN
 IF TG_OP='UPDATE' THEN
  IF TG_TABLE_NAME='group_members' THEN oldorg=(SELECT org_id FROM groups WHERE id=OLD.group_id);ELSE oldorg=OLD.org_id;END IF;
  PERFORM id_record_call_identity(OLD.user_id,(SELECT o.slug FROM organizations o WHERE o.id=oldorg));
 END IF;
 CASE TG_TABLE_NAME
 WHEN 'group_members' THEN
  IF TG_OP='DELETE' THEN actor=OLD.user_id;org=(SELECT org_id FROM groups WHERE id=OLD.group_id);ELSE actor=NEW.user_id;org=(SELECT org_id FROM groups WHERE id=NEW.group_id);END IF;
 WHEN 'user_custom_roles' THEN
  IF TG_OP='DELETE' THEN actor=OLD.user_id;org=OLD.org_id;ELSE actor=NEW.user_id;org=NEW.org_id;END IF;
 WHEN 'memberships' THEN
  IF TG_OP='DELETE' THEN actor=OLD.user_id;org=OLD.org_id;ELSE actor=NEW.user_id;org=NEW.org_id;END IF;
 END CASE;
 slug=(SELECT o.slug FROM organizations o WHERE o.id=org);
 PERFORM id_record_call_identity(actor,slug);
 RETURN NULL;
END $$;
CREATE FUNCTION id_touch_call_identity_group() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE member record; oldslug text; newslug text;
BEGIN
 oldslug=(SELECT slug FROM organizations WHERE id=OLD.org_id);
 IF TG_OP='UPDATE' THEN
  IF TG_TABLE_NAME='groups' THEN
   IF OLD.name IS NOT DISTINCT FROM NEW.name AND OLD.org_id IS NOT DISTINCT FROM NEW.org_id THEN RETURN NEW;END IF;
  ELSE
   IF OLD.name IS NOT DISTINCT FROM NEW.name AND OLD.permissions IS NOT DISTINCT FROM NEW.permissions AND OLD.org_id IS NOT DISTINCT FROM NEW.org_id THEN RETURN NEW;END IF;
  END IF;
  newslug=(SELECT slug FROM organizations WHERE id=NEW.org_id);
 END IF;
 IF TG_TABLE_NAME='groups' THEN
  FOR member IN SELECT user_id FROM group_members WHERE group_id=OLD.id LOOP
   PERFORM id_record_call_identity(member.user_id,oldslug);
   IF newslug IS DISTINCT FROM oldslug THEN PERFORM id_record_call_identity(member.user_id,newslug);END IF;
  END LOOP;
 ELSE
  FOR member IN SELECT user_id FROM user_custom_roles WHERE role_id=OLD.id LOOP
   PERFORM id_record_call_identity(member.user_id,oldslug);
   IF newslug IS DISTINCT FROM oldslug THEN PERFORM id_record_call_identity(member.user_id,newslug);END IF;
  END LOOP;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD;END IF;RETURN NEW;
END $$;
CREATE FUNCTION id_touch_call_identity_owner() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE member record;
BEGIN
 IF TG_TABLE_NAME='users' THEN
  IF TG_OP='UPDATE' AND OLD.status IS NOT DISTINCT FROM NEW.status THEN RETURN NEW;END IF;
  FOR member IN SELECT o.slug FROM memberships m JOIN organizations o ON o.id=m.org_id WHERE m.user_id=OLD.id LOOP
   PERFORM id_record_call_identity(OLD.id,member.slug);
  END LOOP;
 ELSE
  IF TG_OP='UPDATE' AND OLD.status IS NOT DISTINCT FROM NEW.status AND OLD.slug IS NOT DISTINCT FROM NEW.slug THEN RETURN NEW;END IF;
  FOR member IN SELECT user_id FROM memberships WHERE org_id=OLD.id LOOP
   PERFORM id_record_call_identity(member.user_id,OLD.slug);
   IF TG_OP='UPDATE' AND NEW.slug IS DISTINCT FROM OLD.slug THEN PERFORM id_record_call_identity(member.user_id,NEW.slug);END IF;
  END LOOP;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD;END IF;RETURN NEW;
END $$;
CREATE TRIGGER group_members_call_identity AFTER INSERT OR UPDATE OR DELETE ON group_members FOR EACH ROW EXECUTE FUNCTION id_touch_call_identity();
CREATE TRIGGER user_roles_call_identity AFTER INSERT OR UPDATE OR DELETE ON user_custom_roles FOR EACH ROW EXECUTE FUNCTION id_touch_call_identity();
CREATE TRIGGER memberships_call_identity AFTER INSERT OR UPDATE OR DELETE ON memberships FOR EACH ROW EXECUTE FUNCTION id_touch_call_identity();
CREATE TRIGGER groups_call_identity BEFORE UPDATE OR DELETE ON groups FOR EACH ROW EXECUTE FUNCTION id_touch_call_identity_group();
CREATE TRIGGER roles_call_identity BEFORE UPDATE OR DELETE ON custom_roles FOR EACH ROW EXECUTE FUNCTION id_touch_call_identity_group();
CREATE TRIGGER users_call_identity BEFORE UPDATE OR DELETE ON users FOR EACH ROW EXECUTE FUNCTION id_touch_call_identity_owner();
CREATE TRIGGER orgs_call_identity BEFORE UPDATE OR DELETE ON organizations FOR EACH ROW EXECUTE FUNCTION id_touch_call_identity_owner();
CREATE FUNCTION id_assert_call_identity_revoked() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE guarded boolean;pending boolean;
BEGIN
 SELECT enabled INTO guarded FROM identity_call_guard_state WHERE singleton;
 IF NOT guarded THEN RETURN NULL;END IF;
 SELECT version>acknowledged_version INTO pending FROM identity_call_access_fences WHERE subject=NEW.subject AND org_slug=NEW.org_slug;
 IF pending THEN RAISE EXCEPTION 'Document calls must be revoked before committing identity changes' USING ERRCODE='23514';END IF;RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER call_identity_revoked AFTER INSERT OR UPDATE ON identity_call_access_fences DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION id_assert_call_identity_revoked();
