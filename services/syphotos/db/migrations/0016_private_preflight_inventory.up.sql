-- SPDX-License-Identifier: Apache-2.0
-- Existing-source commitments only: no original pin, content key or new private
-- data. Raw legacy metadata is opened from its original SQL row by exact hash.
CREATE TABLE photos_private_storage_preflight_inventory (
 tenant text NOT NULL,owner_sub text NOT NULL,epoch uuid NOT NULL,
 kind text COLLATE "C" NOT NULL CHECK(kind IN('asset','album','edit','album_asset','album_person','media','variant')),
 id text COLLATE "C" NOT NULL,
 size_bytes bigint NOT NULL CHECK(size_bytes>=0 AND size_bytes<=140737488355328),
 sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),
 storage_key text,
 media_payload bytea,
 PRIMARY KEY(tenant,owner_sub,epoch,kind,id),
 FOREIGN KEY(tenant,owner_sub,epoch) REFERENCES photos_private_storage_preflights(tenant,owner_sub,epoch),
 CHECK((kind='variant')=(storage_key IS NOT NULL)),
 CHECK((kind='media')=(media_payload IS NOT NULL)),
 CHECK(media_payload IS NULL OR (octet_length(media_payload) BETWEEN 2 AND 8192 AND size_bytes=octet_length(media_payload) AND sha256=encode(digest(media_payload,'sha256'),'hex'))),
 CHECK(kind='variant' OR size_bytes BETWEEN 2 AND 33554432)
);
ALTER TABLE photos_private_storage_preflight_inventory ENABLE ROW LEVEL SECURITY;
ALTER TABLE photos_private_storage_preflight_inventory FORCE ROW LEVEL SECURITY;
CREATE POLICY photos_private_preflight_inventory_owner ON photos_private_storage_preflight_inventory
 USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true))
 WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE TRIGGER photos_private_preflight_inventory_immutable BEFORE UPDATE OR DELETE ON photos_private_storage_preflight_inventory
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_snapshot_immutable();
CREATE TRIGGER photos_private_preflight_inventory_capture BEFORE INSERT ON photos_private_storage_preflight_inventory
 FOR EACH ROW EXECUTE FUNCTION photos_private_preflight_guard();
