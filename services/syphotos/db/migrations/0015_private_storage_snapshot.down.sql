-- SPDX-License-Identifier: Apache-2.0
DROP TRIGGER IF EXISTS photos_private_people ON album_personas;
DROP TRIGGER IF EXISTS photos_private_members ON album_assets;
DROP TRIGGER IF EXISTS photos_private_edits ON edits;
DROP TRIGGER IF EXISTS photos_private_albums ON albums;
DROP TRIGGER IF EXISTS photos_private_assets ON assets;
DROP FUNCTION IF EXISTS photos_private_row_guard();
DROP POLICY IF EXISTS photos_private_asset_routing ON assets;
DROP POLICY IF EXISTS photos_private_album_routing ON albums;
DROP TABLE IF EXISTS photos_private_storage_preflights;
DROP FUNCTION IF EXISTS photos_private_preflight_guard();
DROP TABLE IF EXISTS photos_private_storage_snapshot;
DROP FUNCTION IF EXISTS photos_private_snapshot_insert_guard();
DROP TABLE IF EXISTS photos_private_storage_states;
