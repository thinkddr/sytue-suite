-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Coedición en tiempo real (bases-2, colaboración C).
--
-- El REGISTRO DE CAMBIOS que siguen las pantallas abiertas es la propia auditoría: toda
-- escritura de Bases ya deja ahí su fila, dentro de su transacción. Lo que le faltaba
-- para poder seguirse sin huecos es el ORDEN: `id` se reparte al insertar, no al
-- confirmar, y una transacción lenta confirmaría una fila con un id menor que otras ya
-- leídas. `xact` es la transacción que la escribió; quien lee solo entrega las filas de
-- transacciones anteriores a la más antigua aún en curso (`pg_snapshot_xmin`), en orden
-- (xact, id): ninguna puede aparecer después por detrás. Funciona tras PgBouncer, que
-- LISTEN/NOTIFY no.
--
-- `op_id` es el identificador que el cliente pone a cada operación de celda: la
-- unicidad hace la operación IDEMPOTENTE (reintentarla tras un corte no la aplica dos
-- veces) y el cliente reconoce el eco de las suyas.
--
-- bigint y no xid8: se compara y se indexa en todas las versiones que servimos.
-- El DEFAULT es volátil y reescribe la tabla una vez; las filas que ya había se quedan
-- con la transacción de esta migración, anterior a cualquier cursor.
ALTER TABLE base_audit
    ADD COLUMN xact bigint NOT NULL DEFAULT (pg_current_xact_id()::text::bigint),
    ADD COLUMN op_id uuid;
CREATE INDEX base_audit_cambios ON base_audit (tenant, base_id, xact, id);
CREATE UNIQUE INDEX base_audit_op ON base_audit (tenant, op_id) WHERE op_id IS NOT NULL;

-- PRESENCIA: quién tiene abierta la base y en qué celda está. Efímera (caduca a los 30 s
-- sin latido), pero en la base de datos y no en memoria: así la ven todas las réplicas
-- sin otro canal entre ellas. Sin claves a tablas ni registros: lo que ya no existe o no
-- se puede leer se filtra al leerla.
CREATE TABLE base_presence (
    tenant    text NOT NULL,
    base_id   uuid NOT NULL,
    -- La pestaña: un identificador aleatorio que pone el cliente.
    conn      text NOT NULL CHECK (conn ~ '^[A-Za-z0-9_-]{8,64}$'),
    sub       text NOT NULL,
    name      text NOT NULL DEFAULT '' CHECK (length(name) <= 200),
    table_id  uuid,
    record_id uuid,
    field_id  uuid,
    seen      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (base_id, conn),
    FOREIGN KEY (tenant, base_id) REFERENCES bases (tenant, id)
        ON DELETE CASCADE ON UPDATE CASCADE DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX base_presence_viva ON base_presence (tenant, base_id, seen);

ALTER TABLE base_presence ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_presence FORCE ROW LEVEL SECURITY;
CREATE POLICY base_presence_tenant_isolation ON base_presence
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_presence_platform_merge ON base_presence
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
