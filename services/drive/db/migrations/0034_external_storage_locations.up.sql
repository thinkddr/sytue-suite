-- SPDX-License-Identifier: Apache-2.0
-- A connection change must not redirect an existing object to another account.
-- Only routing configuration and provider credentials sealed with the service
-- KEK live here. Content keys remain exclusively on authorized devices.
BEGIN;
SELECT set_config('app.maintenance','on',true);

ALTER TABLE external_storage ADD COLUMN location_id uuid NOT NULL DEFAULT gen_random_uuid();

CREATE TABLE external_storage_locations (
    tenant text NOT NULL,
    location_id uuid NOT NULL,
    configuration jsonb NOT NULL,
    legacy boolean NOT NULL DEFAULT false,
    PRIMARY KEY (tenant, location_id)
);
CREATE UNIQUE INDEX external_storage_one_legacy ON external_storage_locations (tenant) WHERE legacy;
ALTER TABLE external_storage_locations ENABLE ROW LEVEL SECURITY;
ALTER TABLE external_storage_locations FORCE ROW LEVEL SECURITY;
CREATE POLICY external_storage_locations_isolation ON external_storage_locations
    USING (tenant=current_setting('app.tenant',true))
    WITH CHECK (tenant=current_setting('app.tenant',true));
CREATE POLICY external_storage_locations_maintenance ON external_storage_locations
    USING (current_setting('app.maintenance',true)='on')
    WITH CHECK (current_setting('app.maintenance',true)='on');
CREATE POLICY external_storage_locations_merge ON external_storage_locations
    USING (current_setting('app.merge',true)='on')
    WITH CHECK (current_setting('app.merge',true)='on');

-- Migrations execute with maintenance enabled, including on an existing tenant.
INSERT INTO external_storage_locations (tenant, location_id, configuration, legacy)
    SELECT tenant, location_id, to_jsonb(external_storage), true FROM external_storage;

CREATE FUNCTION pin_external_storage_location() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF ROW(NEW.kind, NEW.instance_id, NEW.endpoint, NEW.base_path, NEW.region,
               NEW.bucket, NEW.use_ssl, NEW.access_key, NEW.tunnel_site_id,
               NEW.tunnel_resource_id, NEW.tunnel_newt_id, NEW.local_endpoint)
           IS DISTINCT FROM
           ROW(OLD.kind, OLD.instance_id, OLD.endpoint, OLD.base_path, OLD.region,
               OLD.bucket, OLD.use_ssl, OLD.access_key, OLD.tunnel_site_id,
               OLD.tunnel_resource_id, OLD.tunnel_newt_id, OLD.local_endpoint) THEN
            NEW.location_id := gen_random_uuid();
        ELSE
            NEW.location_id := OLD.location_id;
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER external_storage_pin_location BEFORE UPDATE ON external_storage
    FOR EACH ROW EXECUTE FUNCTION pin_external_storage_location();

CREATE FUNCTION archive_external_storage_location() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO external_storage_locations (tenant, location_id, configuration)
        VALUES (NEW.tenant, NEW.location_id, to_jsonb(NEW))
        ON CONFLICT (tenant, location_id) DO UPDATE
            SET configuration=EXCLUDED.configuration;
    RETURN NEW;
END;
$$;
CREATE TRIGGER external_storage_archive_location AFTER INSERT OR UPDATE ON external_storage
    FOR EACH ROW EXECUTE FUNCTION archive_external_storage_location();

COMMIT;
