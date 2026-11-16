-- SPDX-License-Identifier: Apache-2.0
-- Retained Mail migration source, NOT an E2E store. No original is removed.
-- The account fence serializes all legacy private writes, including maintenance
-- SMTP/IMAP/queue/expiry writers and transactions started before the migration.
CREATE TABLE mail_private_storage_states (
    tenant text NOT NULL,
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    owner_sub text NOT NULL CHECK (owner_sub<>''),
    mode text NOT NULL DEFAULT 'legacy' CHECK(mode IN ('legacy','building','frozen','private')),
    epoch uuid,
    started_at timestamptz,
    source_rows bigint NOT NULL DEFAULT 0 CHECK(source_rows>=0),
    source_blobs bigint NOT NULL DEFAULT 0 CHECK(source_blobs>=0 AND source_blobs<=source_rows),
    PRIMARY KEY(tenant,account_id),
    UNIQUE(tenant,owner_sub),
    CHECK((mode='legacy' AND epoch IS NULL AND started_at IS NULL) OR
          (mode<>'legacy' AND epoch IS NOT NULL AND started_at IS NOT NULL))
);
ALTER TABLE mail_private_storage_states ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_private_storage_states FORCE ROW LEVEL SECURITY;
CREATE POLICY mail_private_state_owner ON mail_private_storage_states
    USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.private_mail_owner',true))
    WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.private_mail_owner',true));
CREATE POLICY mail_private_state_maintenance ON mail_private_storage_states
    USING(current_setting('app.maintenance',true)='on')
    WITH CHECK(current_setting('app.maintenance',true)='on');

-- C collation makes the byte-wise cursor order identical to the client. A row
-- contains exact retained JSON bytes OR internal immutable MIME routing. Keys
-- never appear in source pages. Every MIME is hashed before the freeze commits.
CREATE TABLE mail_private_storage_source (
    tenant text NOT NULL,
    account_id uuid NOT NULL,
    owner_sub text NOT NULL,
    epoch uuid NOT NULL,
    kind text COLLATE "C" NOT NULL,
    id text COLLATE "C" NOT NULL,
    payload bytea,
    storage_key text,
    size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 0 AND 140737488355328),
    sha256 text NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'),
    PRIMARY KEY(tenant,account_id,epoch,kind,id),
    FOREIGN KEY(tenant,account_id) REFERENCES mail_private_storage_states(tenant,account_id),
    CHECK((payload IS NULL AND storage_key IS NOT NULL AND kind IN ('mime_message','mime_outbound')) OR
          (payload IS NOT NULL AND storage_key IS NULL AND octet_length(payload)=size_bytes))
);
CREATE INDEX mail_private_source_blob_pins ON mail_private_storage_source(storage_key) WHERE storage_key IS NOT NULL;
ALTER TABLE mail_private_storage_source ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_private_storage_source FORCE ROW LEVEL SECURITY;
CREATE POLICY mail_private_source_read ON mail_private_storage_source FOR SELECT
    USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.private_mail_owner',true));
CREATE POLICY mail_private_source_build ON mail_private_storage_source FOR INSERT
    WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.private_mail_owner',true)
      AND EXISTS(SELECT 1 FROM mail_private_storage_states s WHERE s.tenant=mail_private_storage_source.tenant
        AND s.account_id=mail_private_storage_source.account_id AND s.owner_sub=mail_private_storage_source.owner_sub
        AND s.epoch=mail_private_storage_source.epoch AND s.mode='building'));
CREATE POLICY mail_private_source_maintenance ON mail_private_storage_source FOR SELECT
    USING(current_setting('app.maintenance',true)='on');
-- Intentionally no UPDATE/DELETE policy for the retained source.

-- sytue:private-storage-fence-v1
CREATE CONSTRAINT TRIGGER mail_private_storage_complete AFTER INSERT OR UPDATE ON mail_private_storage_states
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_build_completed();
CREATE FUNCTION mail_private_storage_guard(t text,a uuid) RETURNS void LANGUAGE plpgsql
SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE owner text;
BEGIN
 SELECT subject INTO owner FROM public.accounts WHERE tenant=t AND id=a AND kind='user';
 IF FOUND THEN
  PERFORM public.sytue_private_storage_guard('public.mail_private_storage_states'::regclass,t,owner,a);
 END IF;
END;
$$;
CREATE FUNCTION mail_private_storage_row_guard() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE row_value jsonb; which_row integer; first_row integer;
BEGIN
    IF TG_TABLE_NAME='accounts' AND TG_OP='UPDATE'
       AND (to_jsonb(NEW)->'signature') IS NOT DISTINCT FROM (to_jsonb(OLD)->'signature')
       AND (to_jsonb(NEW)->'tenant')=(to_jsonb(OLD)->'tenant')
       AND (to_jsonb(NEW)->'subject')=(to_jsonb(OLD)->'subject')
       AND (to_jsonb(NEW)->'kind')=(to_jsonb(OLD)->'kind') THEN
        RETURN NEW; -- public address/profile/quota updates do not rewrite private content
    END IF;
    first_row := CASE WHEN TG_OP='UPDATE' THEN 0 ELSE 1 END;
    FOR which_row IN first_row..1 LOOP
        row_value := CASE WHEN which_row=0 OR TG_OP='DELETE' THEN to_jsonb(OLD) ELSE to_jsonb(NEW) END;
        PERFORM public.mail_private_storage_guard(row_value->>'tenant',
            (CASE WHEN TG_TABLE_NAME='accounts' THEN row_value->>'id' ELSE row_value->>'account_id' END)::uuid);
    END LOOP;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER mail_private_accounts_guard BEFORE UPDATE OR DELETE ON accounts FOR EACH ROW EXECUTE FUNCTION mail_private_storage_row_guard();
CREATE TRIGGER mail_private_mailboxes_guard BEFORE INSERT OR UPDATE OR DELETE ON mailboxes FOR EACH ROW EXECUTE FUNCTION mail_private_storage_row_guard();
CREATE TRIGGER mail_private_threads_guard BEFORE INSERT OR UPDATE OR DELETE ON threads FOR EACH ROW EXECUTE FUNCTION mail_private_storage_row_guard();
CREATE TRIGGER mail_private_messages_guard BEFORE INSERT OR UPDATE OR DELETE ON messages FOR EACH ROW EXECUTE FUNCTION mail_private_storage_row_guard();
CREATE TRIGGER mail_private_outbound_guard BEFORE INSERT OR UPDATE OR DELETE ON outbound FOR EACH ROW EXECUTE FUNCTION mail_private_storage_row_guard();
CREATE TRIGGER mail_private_labels_guard BEFORE INSERT OR UPDATE OR DELETE ON mail_labels FOR EACH ROW EXECUTE FUNCTION mail_private_storage_row_guard();
CREATE TRIGGER mail_private_filters_guard BEFORE INSERT OR UPDATE OR DELETE ON mail_filters FOR EACH ROW EXECUTE FUNCTION mail_private_storage_row_guard();
CREATE TRIGGER mail_private_vacation_guard BEFORE INSERT OR UPDATE OR DELETE ON mail_vacation FOR EACH ROW EXECUTE FUNCTION mail_private_storage_row_guard();
CREATE TRIGGER mail_private_vacation_log_guard BEFORE INSERT OR UPDATE OR DELETE ON mail_vacation_log FOR EACH ROW EXECUTE FUNCTION mail_private_storage_row_guard();
CREATE TRIGGER mail_private_acl_guard BEFORE INSERT OR UPDATE OR DELETE ON mailbox_acl FOR EACH ROW EXECUTE FUNCTION mail_private_storage_row_guard();
CREATE TRIGGER mail_private_requests_guard BEFORE INSERT OR UPDATE OR DELETE ON mailbox_peticiones_edicion FOR EACH ROW EXECUTE FUNCTION mail_private_storage_row_guard();
CREATE TRIGGER mail_private_aliases_guard BEFORE INSERT OR UPDATE OR DELETE ON aliases FOR EACH ROW EXECUTE FUNCTION mail_private_storage_row_guard();
CREATE TRIGGER mail_private_smime_guard BEFORE INSERT OR UPDATE OR DELETE ON smime_identities FOR EACH ROW EXECUTE FUNCTION mail_private_storage_row_guard();
-- Routing-only predicates for tenant maintenance. Ordinary owner RLS must not
-- hide a frozen account from cross-account expiry/queue code. No content leaves
-- these functions and SECURITY INVOKER keeps ordinary database privileges.
CREATE FUNCTION mail_private_storage_frozen_account(t text,a uuid) RETURNS boolean LANGUAGE sql STABLE
SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
 SELECT coalesce((SELECT public.sytue_private_storage_frozen('public.mail_private_storage_states'::regclass,t,subject)
  FROM public.accounts WHERE tenant=t AND id=a AND kind='user'),false);
$$;
CREATE FUNCTION mail_private_storage_retains_blob(k text) RETURNS boolean LANGUAGE sql STABLE
SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
 SELECT EXISTS(SELECT 1 FROM public.mail_private_storage_source WHERE storage_key=k);
$$;
CREATE TRIGGER mail_private_state_scope_guard BEFORE UPDATE OR DELETE ON mail_private_storage_states
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_state_scope_guard();
CREATE TRIGGER mail_private_source_immutable BEFORE UPDATE OR DELETE ON mail_private_storage_source
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_snapshot_immutable();
CREATE POLICY mail_private_state_platform_merge ON mail_private_storage_states
 USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
CREATE POLICY mail_private_source_platform_merge ON mail_private_storage_source
 USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
