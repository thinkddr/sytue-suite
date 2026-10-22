-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El color predeterminado pasa a ser «el de cada app», también para las cuentas
-- existentes que seguían el petróleo heredado de la 0107. Una elección explícita
-- del administrador consta como org.branding.color: se conserva aunque sea petróleo.
-- Las elecciones de cada persona, guardadas en su dispositivo, no se cambian.
ALTER TABLE organizations ALTER COLUMN color SET DEFAULT 'app';

UPDATE organizations AS o
SET color = 'app', updated_at = now()
WHERE o.color = 'petroleo'
  AND NOT EXISTS (
      SELECT 1 FROM audit_log AS a
      WHERE a.org_id = o.id AND a.action = 'org.branding.color'
  );
