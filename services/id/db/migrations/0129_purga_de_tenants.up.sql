-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- PURGA DEL CONTENIDO DE UNA ORGANIZACIÓN QUE DESAPARECE (ADR 0054, enmienda del
-- 2026-11-25). Hasta ahora, al purgarse una cuenta personal borrada, su
-- contenido —correo, archivos, fotos...— se quedaba en diez bases y en el
-- almacén de objetos, y el barrido lo avisaba a nivel error para un borrado a
-- mano. Esta tabla es la cola de ese borrado, que ahora hace Sytue ID:
--
--   * una fila por organización a purgar, con su slug (la clave de tenant en las
--     bases de los servicios) copiado: la organización se renombra al purgar la
--     cuenta, y el slug es lo único que los servicios conocen;
--   * `progress` guarda, por servicio, lo que se borró, lo que quedó retenido y
--     por qué, y si sus objetos ya están borrados. Es lo que se reintenta: un
--     servicio terminado no se vuelve a tocar;
--   * `next_attempt_at` es a la vez el reintento con espera creciente y el
--     arriendo entre réplicas (se adelanta al reclamar la fila).
--
-- Sin columna `tenant` (el slug va en `slug`): la lleva el barrido de Sytue ID, no
-- una petición. Aun así es por organización y va con la RLS de 0127 como las demás:
-- el barrido trabaja SIN ámbito declarado, así que la política le deja pasar, y una
-- ruta que algún día la lea confinada a una organización solo verá la suya.

CREATE TABLE tenant_purges (
    org_id          uuid        PRIMARY KEY REFERENCES organizations (id),
    slug            text        NOT NULL UNIQUE
                    CHECK (slug ~ '^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$'),
    reason          text        NOT NULL CHECK (reason IN ('account_deleted', 'dsr_erasure')),
    requested_at    timestamptz NOT NULL DEFAULT now(),
    completed_at    timestamptz,
    attempts        integer     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error      text        NOT NULL DEFAULT '',
    progress        jsonb       NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX tenant_purges_pendientes_idx ON tenant_purges (next_attempt_at)
    WHERE completed_at IS NULL;

ALTER TABLE tenant_purges ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_purges FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_purges_ambito ON tenant_purges
    USING (id_ambito_org(org_id)) WITH CHECK (id_ambito_org(org_id));

-- Las cuentas personales ya purgadas antes de esta migración esperaban su borrado
-- a mano: entran en la cola. Se reconocen por su nombre, que la purga cambió por
-- su id (PurgarCuentaPropia), y por no tener ya a nadie.
INSERT INTO tenant_purges (org_id, slug, reason)
SELECT o.id, o.slug, 'account_deleted'
  FROM organizations o
 WHERE o.kind = 'personal' AND o.name = o.id::text
   AND NOT EXISTS (SELECT 1 FROM memberships m WHERE m.org_id = o.id)
ON CONFLICT DO NOTHING;
