-- SPDX-License-Identifier: Apache-2.0
-- Call keys never enter PostgreSQL: only device ACL, join nonces and room references.
CREATE TABLE matrix_call_members (
 tenant text NOT NULL,
 call_id uuid NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
 subject text NOT NULL,
 device_id text NOT NULL,
 join_id uuid NOT NULL,
 role text NOT NULL CHECK(role IN ('host','speaker','viewer')),
 publish_allowed boolean NOT NULL DEFAULT true,
 pending_until timestamptz NOT NULL,
 joined boolean NOT NULL DEFAULT false,
 livekit_sid text NOT NULL DEFAULT '',
 removed boolean NOT NULL DEFAULT false,
 blocked boolean NOT NULL DEFAULT false,
 ready_generation bigint NOT NULL DEFAULT 0,
 PRIMARY KEY(tenant,call_id,subject)
);
CREATE INDEX matrix_call_members_subject ON matrix_call_members(subject,tenant,call_id);
CREATE TABLE matrix_call_epochs (
 tenant text NOT NULL,
 call_id uuid NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
 generation bigint NOT NULL CHECK(generation>0),
 room_id text NOT NULL UNIQUE,
 livekit_room text NOT NULL UNIQUE,
 members jsonb NOT NULL CHECK(jsonb_typeof(members)='array'),
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant,call_id,generation)
);
ALTER TABLE matrix_call_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_call_members FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_call_members_tenant ON matrix_call_members USING(tenant=current_setting('app.tenant',true)) WITH CHECK(tenant=current_setting('app.tenant',true));
CREATE POLICY matrix_call_members_maintenance ON matrix_call_members USING(current_setting('app.maintenance',true)='on') WITH CHECK(current_setting('app.maintenance',true)='on');
CREATE POLICY matrix_call_members_platform_merge ON matrix_call_members USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
ALTER TABLE matrix_call_epochs ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_call_epochs FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_call_epochs_tenant ON matrix_call_epochs USING(tenant=current_setting('app.tenant',true)) WITH CHECK(tenant=current_setting('app.tenant',true));
CREATE POLICY matrix_call_epochs_maintenance ON matrix_call_epochs USING(current_setting('app.maintenance',true)='on') WITH CHECK(current_setting('app.maintenance',true)='on');
CREATE POLICY matrix_call_epochs_platform_merge ON matrix_call_epochs USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
