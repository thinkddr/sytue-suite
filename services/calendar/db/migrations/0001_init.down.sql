-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP POLICY IF EXISTS invitations_tenant_isolation ON invitations;
DROP POLICY IF EXISTS attendees_tenant_isolation ON attendees;
DROP POLICY IF EXISTS events_tenant_isolation ON events;
DROP POLICY IF EXISTS calobj_tenant_isolation ON calendar_objects;
DROP POLICY IF EXISTS calendars_tenant_isolation ON calendars;

DROP TABLE IF EXISTS events_outbox;
DROP TABLE IF EXISTS invitations;
DROP TABLE IF EXISTS attendees;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS calendar_objects;
DROP TABLE IF EXISTS calendars;
