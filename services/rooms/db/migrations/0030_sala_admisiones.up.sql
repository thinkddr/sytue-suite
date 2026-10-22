-- Sala de espera: llamar a la puerta y que te dejen entrar.
--
-- Bloquear una sala (migración 0029) deja fuera a TODO el mundo, y eso resuelve
-- lo urgente —que no vuelva a entrar quien acabas de echar— pero no el caso
-- normal: llega quien tenía que llegar, se encuentra la puerta cerrada y no hay
-- forma de abrirle a él solo sin reabrirla para todos.
--
-- Una fila por persona que llama. `granted_at` nulo es «esperando»; con fecha,
-- «puede pasar». No se borra al admitir: quien entra, sale y vuelve a entrar no
-- tiene que volver a llamar, que sería la sala de espera más molesta posible.
CREATE TABLE IF NOT EXISTS room_admissions (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant       text        NOT NULL,
    room_id      uuid        NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    -- identity: la misma identidad que en `participants` (= sub de Sytue ID).
    identity     text        NOT NULL,
    -- name: cómo se presenta, para que el anfitrión sepa a quién abre. Es un dato
    -- de PRESENTACIÓN: quien autoriza es `identity`, que viene del token.
    name         text        NOT NULL DEFAULT '',
    requested_at timestamptz NOT NULL DEFAULT now(),
    -- granted_at nulo = esperando; con fecha = admitido.
    granted_at   timestamptz,
    -- Una sola fila por (sala, persona): llamar dos veces no hace dos colas.
    UNIQUE (room_id, identity)
);

-- La cola se lee por sala y solo la parte que espera, que es lo que se pinta.
CREATE INDEX IF NOT EXISTS room_admissions_pendientes_idx
    ON room_admissions (tenant, room_id, requested_at)
    WHERE granted_at IS NULL;

ALTER TABLE room_admissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE room_admissions FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS room_admissions_tenant_isolation ON room_admissions;
CREATE POLICY room_admissions_tenant_isolation ON room_admissions
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Política de fusión de organizaciones. Sin ella el preflight de la fusión
-- aborta, y si alguien la saltara, la cola de la puerta se quedaría en la
-- organización vieja: nadie relacionaría «llamé y no me abren» con una fusión de
-- hace tres semanas.
DROP POLICY IF EXISTS room_admissions_platform_merge ON room_admissions;
CREATE POLICY room_admissions_platform_merge ON room_admissions
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
