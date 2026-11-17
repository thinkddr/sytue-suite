-- SPDX-License-Identifier: Apache-2.0
-- Routing and an opaque encrypted-event reference only. No private contents,
-- user credentials, encryption keys or recovery material are stored here.
CREATE TABLE matrix_storage_cutovers (
 tenant text NOT NULL,
 subject text NOT NULL,
 product text NOT NULL CHECK(product IN('hub','mail','drive','docs','sheets','slides','pdf','calendar','contacts','notes','photos','rooms','cast','erp','bases')),
 epoch uuid NOT NULL,
 phase text NOT NULL CHECK(phase IN('freezing','frozen','isolated')),
 source_rooms jsonb NOT NULL CHECK(jsonb_typeof(source_rooms)='array' AND jsonb_array_length(source_rooms)<=100),
 receipt_room text,
 receipt_event text,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant,subject,product),
 CHECK((phase='isolated' AND receipt_room IS NOT NULL AND receipt_event IS NOT NULL) OR (phase<>'isolated' AND receipt_room IS NULL AND receipt_event IS NULL))
);
ALTER TABLE matrix_storage_cutovers ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_storage_cutovers FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_storage_cutovers_tenant ON matrix_storage_cutovers
 USING(tenant=current_setting('app.tenant',true)) WITH CHECK(tenant=current_setting('app.tenant',true));
CREATE POLICY matrix_storage_cutovers_maintenance ON matrix_storage_cutovers
 USING(current_setting('app.maintenance',true)='on') WITH CHECK(current_setting('app.maintenance',true)='on');
CREATE POLICY matrix_storage_cutovers_platform_merge ON matrix_storage_cutovers
 USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
CREATE FUNCTION matrix_storage_cutover_monotonic() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'matrix_storage_cutover_retained'; END IF;
 IF NEW.tenant<>OLD.tenant OR NEW.subject<>OLD.subject OR NEW.product<>OLD.product
  OR NEW.epoch<>OLD.epoch OR NEW.source_rooms<>OLD.source_rooms OR NEW.created_at<>OLD.created_at
  OR NOT((OLD.phase='freezing' AND NEW.phase IN('freezing','frozen')) OR (OLD.phase='frozen' AND NEW.phase IN('frozen','isolated')) OR (OLD.phase='isolated' AND NEW.phase='isolated'))
  OR (OLD.phase='isolated' AND (NEW.receipt_room IS DISTINCT FROM OLD.receipt_room OR NEW.receipt_event IS DISTINCT FROM OLD.receipt_event))
 THEN RAISE EXCEPTION 'matrix_storage_cutover_immutable'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER matrix_storage_cutover_monotonic BEFORE UPDATE OR DELETE ON matrix_storage_cutovers
 FOR EACH ROW EXECUTE FUNCTION matrix_storage_cutover_monotonic();
