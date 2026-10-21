-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN IF EXISTS(SELECT 1 FROM identity_call_guard_state WHERE enabled) THEN RAISE EXCEPTION 'An enabled identity call fence cannot be discarded';END IF;END $$;
DROP TRIGGER call_identity_revoked ON identity_call_access_fences;
DROP TRIGGER group_members_call_identity ON group_members;
DROP TRIGGER user_roles_call_identity ON user_custom_roles;
DROP TRIGGER memberships_call_identity ON memberships;
DROP TRIGGER groups_call_identity ON groups;
DROP TRIGGER roles_call_identity ON custom_roles;
DROP TRIGGER users_call_identity ON users;
DROP TRIGGER orgs_call_identity ON organizations;
DROP FUNCTION id_assert_call_identity_revoked();
DROP FUNCTION id_touch_call_identity_owner();
DROP FUNCTION id_touch_call_identity_group();
DROP FUNCTION id_touch_call_identity();
DROP FUNCTION id_record_call_identity(uuid,text);
DROP TABLE identity_call_access_fences;
DROP TABLE identity_call_guard_state;
DROP SEQUENCE identity_call_access_version_seq;
