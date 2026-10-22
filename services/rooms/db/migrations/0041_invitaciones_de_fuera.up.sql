-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- LA GENTE DE FUERA ACEPTA ANTES DE ENTRAR (decidido por el dueño el 2026-10-16: «Que
-- acepte primero», también en mensajes directos y grupos).
--
-- Hasta ahora a una persona de OTRA organización (ADR 0040) se la añadía a un canal, un
-- DM o un grupo y entraba directamente. Ahora recibe una invitación y no es miembro
-- —ni lee, ni escribe, ni le llega nada del canal— hasta aceptarla; puede rechazarla.
--
-- Tabla APARTE de channel_members a propósito: así una invitación no puede abrir
-- ninguna de las verjas que miran la pertenencia (requireChannelAccess y las demás).
-- Aceptar es moverla a channel_members; rechazar, borrarla.
--
-- Las personas de fuera que YA están en canales se quedan dentro y cuentan como
-- aceptadas (decisión del dueño): esta migración no toca channel_members.
CREATE TABLE invitaciones_canal (
    tenant        text        NOT NULL,
    channel_id    uuid        NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    -- La persona invitada (de otra organización) y el nombre que dio Sytue ID.
    subject       text        NOT NULL,
    name          text        NOT NULL DEFAULT '',
    -- Quién invita, para que la invitación diga de quién viene.
    invitado_por  text        NOT NULL DEFAULT '',
    nombre_de_quien_invita text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, channel_id, subject)
);

-- «¿Qué invitaciones tengo?»: la persona invitada pregunta desde SU organización.
CREATE INDEX invitaciones_canal_por_persona ON invitaciones_canal (subject);

ALTER TABLE invitaciones_canal ENABLE ROW LEVEL SECURITY;
ALTER TABLE invitaciones_canal FORCE ROW LEVEL SECURITY;

CREATE POLICY invitaciones_canal_tenant_isolation ON invitaciones_canal
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- La invitada las lee desde su organización, que no es la anfitriona: el servidor las
-- busca por la política de mantenimiento, siempre por el `sub` del token verificado
-- (store/invitaciones.go). Solo lectura: aceptar y rechazar van en la anfitriona.
CREATE POLICY invitaciones_canal_maintenance ON invitaciones_canal FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on');

CREATE POLICY invitaciones_canal_platform_merge ON invitaciones_canal
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
