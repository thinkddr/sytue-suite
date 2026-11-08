-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- APARATOS DE iOS A LOS QUE DESPERTAR (ADR 0039 y 0078, enmiendas del 2026-11-06).
--
-- Un único registro para las quince apps nativas: el token que APNs da a CADA app en
-- CADA aparato, con el bundle de la app (el `apns-topic` del envío). El Hub los
-- despierta con un aviso VACÍO (platform/pkg/apns) cuando cambia algo del producto de
-- esa app (la señal de cambios, `datos`), y la app vuelve a leer lo suyo y repinta su
-- widget. Sustituye a `push_apns` de Rooms, que no llegó a tener clientes.
--
-- La clave es (tenant, bundle, token): el token ya identifica a la app en el aparato;
-- re-registrarlo hace UPSERT (el mismo aparato puede cambiar de dueño al reinstalar) y
-- renueva `visto`, que es lo que purga a los olvidados (30 días sin registrarse).
--
-- `ultimo_push` y `pendiente` son el tope de UN despertar cada 5 min por aparato, que
-- tiene que valer con varias réplicas del Hub y por eso vive aquí y no en memoria: un
-- cambio marca `pendiente`, y manda quien consigue RESERVAR la fila (UPDATE … WHERE
-- ultimo_push < now() - 5 min RETURNING); lo que llega dentro de la ventana se queda
-- pendiente y lo recoge el barrido de cada minuto con la misma reserva.
--
-- Seguridad: RLS deny-by-default por tenant. El token NO es público —quien lo tiene
-- puede mandar avisos a ese aparato—, así que se aísla como un secreto y nunca sale en
-- los registros. El barrido y la purga cruzan organizaciones con `app.maintenance`.

CREATE TABLE aparatos_apns (
    tenant      text        NOT NULL,
    subject     text        NOT NULL,
    bundle      text        NOT NULL,   -- bundle id de la app (apns-topic)
    token       text        NOT NULL,   -- token de aparato APNs (hex)
    creado      timestamptz NOT NULL DEFAULT now(),
    visto       timestamptz NOT NULL DEFAULT now(),
    ultimo_push timestamptz,
    pendiente   timestamptz,           -- un cambio que aún no se ha despertado
    PRIMARY KEY (tenant, bundle, token),
    CONSTRAINT aparatos_apns_token_hex CHECK (token ~ '^[0-9A-Fa-f]{32,200}$'),
    CONSTRAINT aparatos_apns_bundle CHECK (bundle ~ '^eu\.sytue\.[a-z]+$')
);

CREATE INDEX aparatos_apns_persona ON aparatos_apns (tenant, subject);
CREATE INDEX aparatos_apns_pendientes ON aparatos_apns (pendiente) WHERE pendiente IS NOT NULL;
CREATE INDEX aparatos_apns_visto ON aparatos_apns (visto);

ALTER TABLE aparatos_apns ENABLE ROW LEVEL SECURITY;
ALTER TABLE aparatos_apns FORCE ROW LEVEL SECURITY;

CREATE POLICY aparatos_apns_tenant_isolation ON aparatos_apns
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- El barrido de pendientes y la purga (tareas del propio Hub, no de una petición).
CREATE POLICY aparatos_apns_maintenance ON aparatos_apns
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

-- Fusión de organizaciones y borrado de cuenta (0003; services/id/internal/merge).
CREATE POLICY aparatos_apns_platform_merge ON aparatos_apns
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
