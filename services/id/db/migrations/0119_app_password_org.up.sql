-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
-- Cada contraseña nueva queda ligada a la organización activa al crearla.
-- NULL solo conserva metadatos de credenciales antiguas ambiguas: el canje las
-- rechaza hasta reconectar. No hay fallback que pueda cambiarles el tenant.
ALTER TABLE app_passwords ADD COLUMN org_id uuid REFERENCES organizations(id) ON DELETE CASCADE;

-- Solo se conoce el destino antiguo si había una única membresía, activa.
-- Contar TODAS evita asignar la credencial de una org suspendida a otra activa.
UPDATE app_passwords a SET org_id = m.org_id
FROM memberships m JOIN organizations o ON o.id = m.org_id
WHERE a.user_id = m.user_id AND o.status = 'active'
AND (SELECT count(*) FROM memberships all_m WHERE all_m.user_id = a.user_id) = 1;

CREATE INDEX app_passwords_org_user_idx ON app_passwords (org_id, user_id) WHERE revoked_at IS NULL;
