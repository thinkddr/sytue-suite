-- SPDX-License-Identifier: Apache-2.0
-- A previous binary cannot mark a canonical source frozen while an admitted
-- publication can still reach that source. This shares the retained evidence
-- and owner policy of the existing origin contract; no content is read.
CREATE FUNCTION matrix_storage_publication_cutover_guard() RETURNS trigger
 LANGUAGE plpgsql SECURITY INVOKER SET search_path=pg_catalog,public AS $$
DECLARE previous_contract text; previous_owner text; previous_product text; pending boolean;
BEGIN
 IF OLD.phase='freezing' AND NEW.phase='frozen' THEN
  previous_contract:=current_setting('app.matrix_publication_contract',true);
  previous_owner:=current_setting('app.matrix_authority_owner',true);
  previous_product:=current_setting('app.matrix_authority_product',true);
  PERFORM set_config('app.matrix_publication_contract','1',true),
   set_config('app.matrix_authority_owner',NEW.subject,true),
   set_config('app.matrix_authority_product',NEW.product,true);
  SELECT EXISTS(SELECT 1 FROM public.matrix_storage_publications
   WHERE tenant=NEW.tenant AND subject=NEW.subject AND product=NEW.product
    AND room_id IN(SELECT jsonb_array_elements_text(NEW.source_rooms)) AND status='sending') INTO pending;
  PERFORM set_config('app.matrix_publication_contract',COALESCE(previous_contract,''),true),
   set_config('app.matrix_authority_owner',COALESCE(previous_owner,''),true),
   set_config('app.matrix_authority_product',COALESCE(previous_product,''),true);
  IF pending THEN RAISE EXCEPTION 'canonical publication origin is still pending' USING ERRCODE='55000'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER matrix_storage_publication_cutover_guard BEFORE UPDATE
 ON matrix_storage_cutovers FOR EACH ROW EXECUTE FUNCTION matrix_storage_publication_cutover_guard();
