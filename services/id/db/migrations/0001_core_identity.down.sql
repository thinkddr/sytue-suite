-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP TRIGGER IF EXISTS oauth_clients_set_updated_at ON oauth_clients;
DROP TRIGGER IF EXISTS users_set_updated_at ON users;
DROP TRIGGER IF EXISTS organizations_set_updated_at ON organizations;
DROP FUNCTION IF EXISTS set_updated_at();

DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS totp_secrets;
DROP TABLE IF EXISTS webauthn_challenges;
DROP TABLE IF EXISTS webauthn_credentials;
DROP TABLE IF EXISTS signing_keys;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS auth_codes;
DROP TABLE IF EXISTS oauth_clients;
DROP TABLE IF EXISTS group_members;
DROP TABLE IF EXISTS groups;
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS organizations;
