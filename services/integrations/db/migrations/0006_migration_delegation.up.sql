-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- MIGRACIÓN A LA CUENTA DE CADA PERSONA (no a la del admin).
--
-- Hasta ahora un trabajo de migración escribía en Sytue con el Bearer del admin que
-- lo lanzaba: todo aterrizaba en SU cuenta. Ahora la pasarela canjea (RFC 8693, token
-- exchange de Sytue ID) un token ACOTADO por cada usuario objetivo y escribe con él,
-- así que el contenido cae donde tiene que caer.
--
--   migration_delegates  las credenciales del client DELEGABLE de la organización
--                        (mig 0025 de services/id) con las que se hace el canje. El
--                        secreto va sellado con la KEK, igual que instance_secrets.
--
-- POR QUÉ UNA FILA POR ORGANIZACIÓN Y NO UNA CONFIG GLOBAL: un client delegable
-- puede actuar como cualquier miembro de SU organización. Un secreto único y global
-- sería una llave maestra de impersonación de toda la plataforma; el client global de
-- la pasarela (`sytue-gateway`, org_id NULL) por eso no puede ser delegable, y el
-- CHECK de la mig 0025 de services/id lo prohíbe por construcción.
--
-- QUIÉN LA ESCRIBE: SOLO el operador de plataforma, por PUT /operator/delegated-client
-- con la credencial de operador del despliegue — nunca un admin de organización desde
-- la Console. Es la contraparte de que `may_impersonate` en Sytue ID tampoco se activa
-- por ninguna API. Dos credenciales de plataforma, ninguna de usuario.

CREATE TABLE migration_delegates (
    org_id            text        PRIMARY KEY,
    client_id         text        NOT NULL,
    -- Sellado con la KEK (AES-256-GCM, prefijo "encv1:"). Nunca se serializa.
    client_secret_enc text        NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE migration_delegates ENABLE ROW LEVEL SECURITY;
ALTER TABLE migration_delegates FORCE ROW LEVEL SECURITY;

CREATE POLICY migration_delegates_org_isolation ON migration_delegates
    USING (org_id = current_setting('app.tenant', true))
    WITH CHECK (org_id = current_setting('app.tenant', true));

GRANT SELECT, INSERT, UPDATE, DELETE ON migration_delegates TO integrations_app;

-- ---------------------------------------------------------------------------
-- migration_jobs: el destino ya NO es quien lanza el trabajo.
--
--   subject → target_user : el usuario de Sytue EN CUYA CUENTA aterriza el
--                           contenido (correo o uuid). Antes era el `sub` del JWT
--                           del admin; ahora lo elige él en el asistente.
--   dest_token_enc        : desaparece. El token de escritura se canjea al vuelo
--                           por usuario y dura 5 minutos: guardar un token de
--                           sesión de admin durante horas era, además, un riesgo
--                           que ya no hace falta correr.
--
-- sytue:destructive-ok conservar dest_token_enc un despliegue más, como pediría el
-- expand/contract, es dejar vivos justo los tokens de admin de larga vida que esta
-- migración retira: el riesgo de seguridad pesa más que la ventana de rollout. Si hay
-- que volver atrás, se restaura el snapshot previo (operations.md §9.1).
-- ---------------------------------------------------------------------------
ALTER TABLE migration_jobs RENAME COLUMN subject TO target_user;
ALTER TABLE migration_jobs DROP COLUMN dest_token_enc;
