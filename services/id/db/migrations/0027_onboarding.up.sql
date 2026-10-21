-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Alta inicial guiada (el PRIMER contacto de un cliente con Sytue). Dos tablas, una
-- por naturaleza del dato:
--
--   1) onboarding_progress — el ESTADO del asistente, por USUARIO (no por org: la
--      org todavía no existe en el primer paso). Es lo que permite retomar donde se
--      dejó si cierra el navegador. Guarda el mínimo imprescindible (modo, paso,
--      la org que se creó y cuándo se terminó): TODO lo demás —nombre, dominio,
--      facturación, invitaciones, roles— se lee de su tabla real, nunca de un
--      borrador. Así el resumen final refleja lo que EXISTE, no lo que se tecleó.
--
--   2) org_profiles — datos de la ORGANIZACIÓN que hasta ahora no tenían sitio: el
--      dominio propio con su verificación DNS y los datos fiscales/de facturación.
--      No van en `organizations` (tabla caliente del login) ni en el borrador del
--      asistente: son datos duraderos de la org, y cualquier superficie futura
--      (facturación, correo del dominio propio) los lee de aquí.
--
-- AISLAMIENTO POR TENANT: igual que el resto (mig 0001/0019), a nivel de aplicación
-- con FK + scoping por org/usuario en CADA query, deny-by-default. El handler exige
-- ser OWNER de la org referenciada por su propia fila de onboarding: un org_id ajeno
-- no autoriza nada.

CREATE TABLE onboarding_progress (
    user_id      uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    -- Org creada durante el alta. SET NULL (no CASCADE): si la org se borra, el
    -- rastro del alta del usuario no desaparece con ella.
    org_id       uuid REFERENCES organizations (id) ON DELETE SET NULL,
    -- 'individual' | 'organization'. El recorrido individual salta personas y roles;
    -- por debajo crea igualmente una organización de una persona (modelo único).
    mode         text NOT NULL DEFAULT '',
    -- Último paso alcanzado (identificador del paso del asistente). Validado contra
    -- el recorrido del modo en el handler: nunca texto libre del navegador.
    step         text NOT NULL DEFAULT '',
    completed_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER onboarding_progress_set_updated_at BEFORE UPDATE ON onboarding_progress
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE org_profiles (
    org_id             uuid PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    -- Dominio propio reclamado (vacío = la org usa su subdominio de la suite).
    domain             text NOT NULL DEFAULT '',
    -- Valor aleatorio del registro TXT de verificación (_sytue.<dominio>). Se
    -- regenera si el dominio cambia: un token no vale para otro dominio.
    domain_token       text NOT NULL DEFAULT '',
    -- Sello de la verificación DNS REAL (consulta TXT). NULL = sin verificar; que
    -- sea NULL o no lo decide el resolver, jamás el botón del navegador.
    domain_verified_at timestamptz,
    -- Datos fiscales/de facturación. Opcionales por diseño: no bloquean el alta.
    legal_name         text NOT NULL DEFAULT '',
    tax_id             text NOT NULL DEFAULT '',
    address            text NOT NULL DEFAULT '',
    billing_email      text NOT NULL DEFAULT '',
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

-- Un dominio VERIFICADO pertenece a UNA sola organización. El índice cubre solo lo
-- verificado a propósito: reclamar un dominio no prueba nada (cualquiera puede
-- escribir «acme.com» en el asistente), así que una reclamación sin verificar NO
-- puede bloquear al dueño real — sería un secuestro trivial. Quien publica el TXT
-- gana; el segundo intento de verificar el mismo dominio choca aquí.
CREATE UNIQUE INDEX org_profiles_domain_idx ON org_profiles (domain)
    WHERE domain <> '' AND domain_verified_at IS NOT NULL;

CREATE TRIGGER org_profiles_set_updated_at BEFORE UPDATE ON org_profiles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
