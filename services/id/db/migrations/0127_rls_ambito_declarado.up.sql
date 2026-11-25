-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- RLS EN SYTUE ID: el ámbito que la petición DECLARA lo hace cumplir la base.
--
-- Hasta aquí solo `identity_call_guard_state` tenía RLS (0117). El resto del esquema
-- se aislaba únicamente con el `WHERE org_id = $1` de cada consulta: un predicado
-- olvidado en una ruta de administración era una fuga entre organizaciones que nada
-- más paraba.
--
-- POR QUÉ NO ES LA MISMA POLÍTICA QUE EN LOS SERVICIOS DE DATOS. Allí toda consulta
-- de negocio tiene un tenant (`app.tenant`) y sin él no se ve nada. Sytue ID no puede
-- funcionar así: es quien DECIDE la organización. Iniciar sesión busca a la persona
-- por correo sin saber aún en qué organización está, el token endpoint resuelve un
-- código por su hash, SCIM resuelve el token antes de saber de quién es, y la consola
-- de plataforma y la fusión cruzan organizaciones por diseño. Una política que negara
-- todo sin ámbito habría parado el IdP entero.
--
-- Así que el contrato es otro, y es estricto donde se declara:
--
--   - Sin ámbito (`app.org_id`, `app.tenant` y `app.user_id` sin fijar o vacíos), la
--     política deja pasar: los flujos globales del IdP siguen como estaban.
--   - CON ámbito, la base solo deja ver y escribir filas de esa organización y/o de
--     esa persona, también al INSERTAR y al ACTUALIZAR (WITH CHECK). Lo fija el
--     servidor, no el SQL de cada método: la API de administración se confina a la
--     organización ACTIVA del token y las rutas de seguridad de la cuenta a la persona
--     del token (services/id/internal/store/ambito.go). Un `WHERE` que falte en esas
--     rutas ya no saca nada de otra organización ni de otra persona.
--
-- FORCE y no solo ENABLE: el rol de la aplicación es el dueño de las tablas (migra
-- con su propio DSN) y ENABLE no se le aplica al dueño (ver pgtest.ExigirRLSForzada).
--
-- Los DISPARADORES ven todo (`pg_trigger_depth() > 0`). Mantienen invariantes del
-- propio esquema que cruzan ámbitos por definición: suspender a una persona desde la
-- consola de UNA organización tiene que dejar la valla de llamadas de documentos en
-- TODAS las suyas (0117), y con la política aplicada dentro del disparador solo se
-- habría escrito la de la organización activa —una revocación a medias que no falla—.
-- Lo que se escribe desde un disparador lo decide el esquema, no la petición.
--
-- Fuera, a propósito:
--   - `users` y `organizations`: son el directorio global de identidades y de
--     organizaciones. Una persona pertenece a varias organizaciones; lo que es de una
--     organización es la PERTENENCIA (`memberships`), y esa sí se confina.
--   - `identity_call_access_fences`: contabilidad interna que solo escriben los
--     disparadores de 0117.
--   - `usernames_retirados` (0126): es la reserva GLOBAL de los @usuario que alguien
--     dejó, como la unicidad de `users.username`. Cambiar de @usuario pregunta si el
--     nuevo lo tiene reservado OTRA persona; confinarla a la persona que pregunta
--     haría que la reserva ajena no se viera y se pudiera quitar el nombre a quien
--     lo dejó hace un rato.
--   - Las tablas sin organización ni persona (claves de firma, geoip, límites…).

-- El ámbito activo. `nullif(…, '')`: un SET LOCAL de una transacción anterior deja la
-- variable definida y vacía en la sesión, y vacío es «sin ámbito», no un uuid malo.
CREATE FUNCTION id_ambito_org(o uuid) RETURNS boolean
    LANGUAGE sql STABLE PARALLEL RESTRICTED
    RETURN pg_trigger_depth() > 0
        OR nullif(current_setting('app.org_id', true), '') IS NULL
        OR o = nullif(current_setting('app.org_id', true), '')::uuid;

CREATE FUNCTION id_ambito_persona(u uuid) RETURNS boolean
    LANGUAGE sql STABLE PARALLEL RESTRICTED
    RETURN pg_trigger_depth() > 0
        OR nullif(current_setting('app.user_id', true), '') IS NULL
        OR u = nullif(current_setting('app.user_id', true), '')::uuid;

-- Las dos tablas de la bóveda y del perfil de Apple guardan la organización por su
-- slug (`tenant`), como los servicios de datos.
CREATE FUNCTION id_ambito_tenant(t text) RETURNS boolean
    LANGUAGE sql STABLE PARALLEL RESTRICTED
    RETURN pg_trigger_depth() > 0
        OR nullif(current_setting('app.tenant', true), '') IS NULL
        OR t = nullif(current_setting('app.tenant', true), '');

-- Por organización.
DO $$
DECLARE tabla text;
BEGIN
    FOREACH tabla IN ARRAY ARRAY[
        'access_policies', 'audit_log', 'audit_sinks', 'bulk_jobs', 'custom_roles',
        'dcr_initial_tokens', 'domain_validation_challenges', 'groups',
        'identity_providers', 'oauth_clients', 'org_branding', 'org_invitations',
        'org_mail_domains', 'org_profiles', 'org_renames', 'org_slug_claims',
        'org_subscriptions', 'saml_idp_keys', 'saml_sps', 'scim_tokens', 'seat_grants',
        'sensitivity_labels', 'subscription_payments'
    ] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', tabla);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', tabla);
        EXECUTE format('CREATE POLICY %I ON %I USING (id_ambito_org(org_id)) WITH CHECK (id_ambito_org(org_id))',
                       tabla || '_ambito', tabla);
    END LOOP;
END $$;

-- Por organización Y por persona.
DO $$
DECLARE tabla text;
BEGIN
    FOREACH tabla IN ARRAY ARRAY[
        'app_passwords', 'auth_codes', 'devices', 'memberships', 'mfa_continuations',
        'onboarding_progress', 'org_mailboxes', 'refresh_tokens', 'role_activations',
        'role_eligibilities', 'sessions', 'user_custom_roles'
    ] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', tabla);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', tabla);
        EXECUTE format('CREATE POLICY %I ON %I'
                       ' USING (id_ambito_org(org_id) AND id_ambito_persona(user_id))'
                       ' WITH CHECK (id_ambito_org(org_id) AND id_ambito_persona(user_id))',
                       tabla || '_ambito', tabla);
    END LOOP;
END $$;

-- Por persona. `group_members` no lleva organización propia: la hereda del grupo, y
-- el grupo sí está confinado, así que desde un ámbito de organización no se llega a
-- un grupo ajeno por el que preguntar.
DO $$
DECLARE tabla text;
BEGIN
    FOREACH tabla IN ARRAY ARRAY[
        'consentimientos_nativos', 'e2e_unlock_envelopes', 'email_verification_tokens',
        'federated_identities', 'group_members', 'mfa_recovery_codes',
        'password_reset_tokens', 'phone_change_codes', 'platform_activations',
        'platform_admins', 'totp_secrets', 'user_avatars', 'user_rubricas',
        'webauthn_challenges', 'webauthn_credentials'
    ] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', tabla);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', tabla);
        EXECUTE format('CREATE POLICY %I ON %I USING (id_ambito_persona(user_id)) WITH CHECK (id_ambito_persona(user_id))',
                       tabla || '_ambito', tabla);
    END LOOP;
END $$;

-- Por organización (slug) Y por persona.
DO $$
DECLARE tabla text;
BEGIN
    FOREACH tabla IN ARRAY ARRAY['apple_profile_links', 'e2e_vault_keys'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', tabla);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', tabla);
        EXECUTE format('CREATE POLICY %I ON %I'
                       ' USING (id_ambito_tenant(tenant) AND id_ambito_persona(user_id))'
                       ' WITH CHECK (id_ambito_tenant(tenant) AND id_ambito_persona(user_id))',
                       tabla || '_ambito', tabla);
    END LOOP;
END $$;
