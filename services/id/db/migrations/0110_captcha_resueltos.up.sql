-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Captchas ALTCHA ya resueltos, COMPARTIDOS entre réplicas (ID-01).
--
-- El verificador ALTCHA recordaba las soluciones usadas en la memoria del proceso
-- (altcha.enMemoria): con una sola réplica basta, pero Sytue ID corre con 2-12
-- (deploy/helm/sytue-one/values-prod.yaml), así que una misma solución valía una
-- vez POR RÉPLICA y un bot amortizaba el coste del proof-of-work hasta N veces
-- reenviando la solución al captcha de /login y /signup. Esta tabla lleva la
-- comprobación de repetición a un sitio común a todas las réplicas, igual que ya
-- hacen Sytue Web (altcha_used) y Sytue Bases (base_altcha_used): INSERT … ON
-- CONFLICT DO NOTHING, y un barrido de las caducadas (store.CleanupExpired).
--
-- Sin columna `tenant`: el captcha es anónimo (alta y login, antes de haber
-- sesión), así que no entra en la fusión de organizaciones (migcheck no pide
-- política de fusión).
CREATE TABLE captcha_used (
    signature  text PRIMARY KEY,
    expires_at timestamptz NOT NULL
);
CREATE INDEX captcha_used_caducadas ON captcha_used (expires_at);

COMMENT ON TABLE captcha_used IS
    'Firmas de retos ALTCHA ya resueltos, para que una solución no se reuse entre réplicas (ID-01).';
