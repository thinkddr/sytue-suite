-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Invitaciones de una organización: un admin invita por email y el invitado
-- acepta fijando su contraseña. Mismo modelo de token que password_reset_tokens
-- (mig 0009): solo se persiste el HASH SHA-256 del token (nunca el token en
-- claro), el token es además FIRMADO (HMAC del servidor) y de un solo uso — se
-- BORRA al aceptarlo (DELETE ... RETURNING atómico) — con caducidad por
-- expires_at. La fila lleva el destino (org + rol) para materializar la membresía
-- al aceptar, sin confiar en ningún dato del navegador.
CREATE TABLE org_invitations (
    token_hash  text PRIMARY KEY,                                 -- SHA-256 (base64url) del token
    org_id      uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    email       text NOT NULL,                                    -- destinatario (normalizado a minúsculas)
    role        text NOT NULL,                                    -- rol a conceder al aceptar (owner|admin|member|billing)
    inviter_id  uuid REFERENCES users (id) ON DELETE SET NULL,    -- quién invitó (auditoría); SET NULL si se da de baja
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- org_email_idx: al reinvitar el mismo correo se purgan las invitaciones previas
-- de esa (org, email) — solo el último enlace vale — por Index Scan, sin Seq Scan.
CREATE INDEX org_invitations_org_email_idx ON org_invitations (org_id, lower(email));
-- expires_idx: purga periódica de las caducadas por Index Scan.
CREATE INDEX org_invitations_expires_idx ON org_invitations (expires_at);
