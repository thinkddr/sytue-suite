-- SPDX-License-Identifier: Apache-2.0
-- A rollback may remove an unused fence, never retained migration input.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM calendar_private_storage_states WHERE mode<>'legacy') THEN
  RAISE EXCEPTION 'calendar_private_storage_retained_source_exists';
 END IF;
END $$;
DROP TRIGGER calendar_private_calendars_guard ON calendars;
DROP TRIGGER calendar_private_objects_guard ON calendar_objects;
DROP TRIGGER calendar_private_events_guard ON events;
DROP TRIGGER calendar_private_attendees_guard ON attendees;
DROP TRIGGER calendar_private_invitations_guard ON invitations;
DROP TRIGGER calendar_private_reminders_guard ON calendar_reminders;
DROP TRIGGER calendar_private_subscriptions_guard ON calendar_subscriptions;
DROP TRIGGER calendar_private_shares_guard ON calendar_shares;
DROP TRIGGER calendar_private_requests_guard ON calendar_peticiones_edicion;
DROP TRIGGER calendar_private_resources_guard ON calendar_resources;
DROP TRIGGER calendar_private_outbox_guard ON events_outbox;
DROP FUNCTION calendar_private_row_guard();
DROP FUNCTION calendar_private_identity_guard(text,text);
DROP FUNCTION calendar_private_lock_owner(text);
DROP FUNCTION calendar_private_lock_identities(text,text[]);
DROP FUNCTION calendar_private_calendar_frozen(text,uuid);
DROP POLICY calendars_private_routing ON calendars;
DROP POLICY events_private_routing ON events;
DROP POLICY calendar_objects_private_routing ON calendar_objects;
DROP POLICY calendar_resources_private_routing ON calendar_resources;
DROP TABLE calendar_private_storage_snapshot;
DROP TABLE calendar_private_storage_aliases;
DROP FUNCTION calendar_private_capture_insert_guard();
DROP TABLE calendar_private_storage_states;
