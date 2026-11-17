-- Public delivery aliases remain mutable after private capture. Their retained
-- originals stay in mail_private_storage_source and are never rewritten here.
CREATE FUNCTION mail_private_alias_routing_guard() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE value jsonb; which_row integer; first_row integer; source_mode text;
BEGIN
    -- Moving an existing identity across owners remains forbidden when either
    -- owner's retained source is frozen. Normal delivery CRUD never does this.
    IF TG_OP='UPDATE' AND (
       NEW.id IS DISTINCT FROM OLD.id OR NEW.tenant IS DISTINCT FROM OLD.tenant
       OR NEW.account_id IS DISTINCT FROM OLD.account_id) THEN
        PERFORM public.mail_private_storage_guard(OLD.tenant,OLD.account_id);
        PERFORM public.mail_private_storage_guard(NEW.tenant,NEW.account_id);
    END IF;
    first_row := CASE WHEN TG_OP='UPDATE' THEN 0 ELSE 1 END;
    FOR which_row IN first_row..1 LOOP
        value := CASE WHEN which_row=0 OR TG_OP='DELETE' THEN to_jsonb(OLD) ELSE to_jsonb(NEW) END;
        -- The capture takes FOR UPDATE on this same state. A routing change
        -- waits until that snapshot commits; it cannot alter a half-built source.
        SELECT s.mode INTO source_mode FROM public.mail_private_storage_states s
        WHERE s.tenant=value->>'tenant' AND s.account_id=(value->>'account_id')::uuid
        FOR KEY SHARE OF s;
        IF source_mode='building' THEN
            RAISE EXCEPTION 'private Mail capture is in progress' USING ERRCODE='PST01';
        END IF;
    END LOOP;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER mail_private_aliases_guard ON aliases;
CREATE TRIGGER mail_private_aliases_guard BEFORE INSERT OR UPDATE OR DELETE ON aliases
 FOR EACH ROW EXECUTE FUNCTION mail_private_alias_routing_guard();
