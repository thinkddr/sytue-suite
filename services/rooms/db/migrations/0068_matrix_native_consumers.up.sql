-- SPDX-License-Identifier: Apache-2.0
-- Bind the receiving native app before opening the browser, independently of
-- the Drive data product. Existing grants keep their original destination.
ALTER TABLE matrix_native_consents ADD COLUMN native_consumer text NOT NULL DEFAULT ''
    CONSTRAINT matrix_native_consents_native_consumer_check CHECK (native_consumer = '' OR (product = 'drive' AND native_consumer IN
      ('hub','mail','docs','sheets','slides','pdf','calendar','contacts','notes','photos','rooms','cast','erp','bases')));

-- Rolling-deploy peers still use SELECT/UPDATE/DELETE without native_consumer.
-- Maintenance is deliberately not an escape hatch for an older native broker.
CREATE POLICY matrix_native_consents_consumer_contract ON matrix_native_consents
    AS RESTRICTIVE
    USING(native_consumer='' OR current_setting('app.matrix_native_consumer_contract',true)='1')
    WITH CHECK(native_consumer='' OR current_setting('app.matrix_native_consumer_contract',true)='1');

-- An older lifecycle writer cannot see consumer rows through the new fence.
-- Retire them atomically when that writer persists a durable denial, so a later
-- reactivation cannot revive an earlier browser ticket. This runs as the caller;
-- it adds no database privilege and restores its transaction-local contract.
CREATE FUNCTION matrix_native_consumers_retire() RETURNS trigger
LANGUAGE plpgsql SECURITY INVOKER SET search_path=pg_catalog,public AS $$
DECLARE previous_contract text;
BEGIN
    IF NEW.status='active' THEN RETURN NEW; END IF;
    previous_contract:=current_setting('app.matrix_native_consumer_contract',true);
    PERFORM set_config('app.matrix_native_consumer_contract','1',true);
    IF TG_TABLE_NAME='matrix_identity_access' THEN
        DELETE FROM public.matrix_native_consents WHERE subject=NEW.subject AND native_consumer<>'';
    ELSE
        DELETE FROM public.matrix_native_consents WHERE tenant=NEW.tenant AND subject=NEW.subject AND native_consumer<>'';
    END IF;
    PERFORM set_config('app.matrix_native_consumer_contract',COALESCE(previous_contract,''),true);
    RETURN NEW;
END $$;
CREATE TRIGGER matrix_native_consumers_retire_identity
    AFTER INSERT OR UPDATE OF status ON matrix_identity_access
    FOR EACH ROW EXECUTE FUNCTION matrix_native_consumers_retire();
CREATE TRIGGER matrix_native_consumers_retire_tenant
    AFTER INSERT OR UPDATE OF status ON matrix_tenant_access
    FOR EACH ROW EXECUTE FUNCTION matrix_native_consumers_retire();
