-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Quita las políticas de ámbito de 0127 y deja esas tablas como estaban: sin RLS.
DO $$
DECLARE tabla text;
BEGIN
    FOREACH tabla IN ARRAY ARRAY[
        'access_policies', 'audit_log', 'audit_sinks', 'bulk_jobs', 'custom_roles',
        'dcr_initial_tokens', 'domain_validation_challenges', 'groups',
        'identity_providers', 'oauth_clients', 'org_branding', 'org_invitations',
        'org_mail_domains', 'org_profiles', 'org_renames', 'org_slug_claims',
        'org_subscriptions', 'saml_idp_keys', 'saml_sps', 'scim_tokens', 'seat_grants',
        'sensitivity_labels', 'subscription_payments',
        'app_passwords', 'auth_codes', 'devices', 'memberships', 'mfa_continuations',
        'onboarding_progress', 'org_mailboxes', 'refresh_tokens', 'role_activations',
        'role_eligibilities', 'sessions', 'user_custom_roles',
        'consentimientos_nativos', 'e2e_unlock_envelopes', 'email_verification_tokens',
        'federated_identities', 'group_members', 'mfa_recovery_codes',
        'password_reset_tokens', 'phone_change_codes', 'platform_activations', 'platform_admins',
        'totp_secrets', 'user_avatars', 'user_rubricas', 'webauthn_challenges',
        'webauthn_credentials',
        'apple_profile_links', 'e2e_vault_keys'
    ] LOOP
        EXECUTE format('DROP POLICY IF EXISTS %I ON %I', tabla || '_ambito', tabla);
        EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', tabla);
        EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', tabla);
    END LOOP;
END $$;

DROP FUNCTION id_ambito_tenant(text);
DROP FUNCTION id_ambito_persona(uuid);
DROP FUNCTION id_ambito_org(uuid);
