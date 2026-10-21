-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Rotación de la KEK (SECRET_ENC_KEY) sin ventana de parada.
--
-- `env_vars.key_version` existe desde 0008 pero NADIE la leía ni la escribía: el
-- versionado estaba a medias y, en la práctica, la clave de cifrado de Sytue Cloud
-- no se podía cambiar sin dejar ilegible todo lo ya guardado. Faltaba la misma
-- columna en las otras dos tablas que guardan secretos —managed_services.creds_enc
-- y projects.webhook_secret_enc/repo_creds_enc—, porque mientras UNA sola fila siga
-- cifrada con la clave vieja, la clave vieja no se puede retirar.
--
-- INVARIANTE de la columna: es una COTA INFERIOR conservadora de la versión con la
-- que están cifrados los blobs de la fila, no un dato exacto. Solo la sube el
-- re-sellado (store.RotateSecretKeys), y solo DESPUÉS de re-cifrar todos los blobs
-- de esa fila. Las escrituras normales no la tocan: una fila nueva nace con
-- DEFAULT 1 aunque se haya cifrado con una versión posterior, y el siguiente pase
-- de rotación la re-sella (sin efecto real) y la pone al día. Eso hace que la
-- corrección NO dependa de que cada camino de escritura futuro se acuerde de fijar
-- la versión —que es justo donde esto se rompería—: si la columna se equivoca, se
-- equivoca por lo bajo, y lo peor que pasa es re-sellar de más.
--
-- DEFAULT 1 es correcto para las filas existentes: hasta ahora solo había una KEK,
-- y el anillo versionado (crypto.NewSealer) trata una clave suelta como la 1.

ALTER TABLE managed_services ADD COLUMN IF NOT EXISTS key_version smallint NOT NULL DEFAULT 1;
ALTER TABLE projects         ADD COLUMN IF NOT EXISTS key_version smallint NOT NULL DEFAULT 1;

-- El re-sellado es CROSS-TENANT (rota los secretos de toda la plataforma) y corre
-- con el rol de aplicación, sin BYPASSRLS. projects y managed_services ya tienen su
-- política PERMISSIVE de mantenimiento (0003); env_vars no la tenía porque hasta
-- ahora ningún proceso de plano de control la tocaba, y sin ella el UPDATE del
-- re-sellado afectaría a 0 filas en silencio. Mismo patrón que 0002/0003/0009: solo
-- abre con app.maintenance='on' (pg.InMaintenanceTx), que las transacciones de
-- negocio no fijan jamás, así que el aislamiento por tenant queda intacto.
CREATE POLICY env_vars_maintenance ON env_vars
    USING (current_setting('app.maintenance', true) = 'on');
