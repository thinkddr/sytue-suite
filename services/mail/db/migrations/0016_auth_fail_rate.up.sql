-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- ---------------------------------------------------------------------------
-- Rate-limit de FALLOS DE AUTENTICACIÓN por IP, para submission (587/465) e
-- IMAP (993). Mismo patrón que `inbound_rate`: ventana de minuto sobre Postgres,
-- sin Redis/Valkey.
--
-- POR QUÉ UNA TABLA APARTE Y NO REUSAR inbound_rate. Cuentan cosas distintas y
-- con umbrales distintos: el MX cuenta MENSAJES de otros servidores (60/min por
-- defecto, y un MTA legítimo puede acercarse), esto cuenta CONTRASEÑAS FALLIDAS
-- de un cliente (10/min, y un cliente legítimo no se acerca ni de lejos).
-- Mezclarlas haría que un pico de correo entrante bloqueara el acceso a los
-- buzones desde esa IP, o al revés.
--
-- Sin RLS, igual que inbound_rate: es infraestructura, no dato de inquilino, y
-- se consulta antes de saber quién es el usuario.
-- ---------------------------------------------------------------------------
CREATE TABLE auth_fail_rate (
    ip          inet        NOT NULL,
    window_min  timestamptz NOT NULL,                   -- truncado a minuto
    count       int         NOT NULL DEFAULT 0,
    PRIMARY KEY (ip, window_min)
);
CREATE INDEX auth_fail_rate_window_idx ON auth_fail_rate (window_min);
