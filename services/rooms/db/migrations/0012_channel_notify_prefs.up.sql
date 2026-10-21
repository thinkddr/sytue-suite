-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- PREFERENCIAS DE NOTIFICACIÓN (Ola 13).
--
-- 1) Nivel de aviso por canal y usuario (channel_members.notify_level): 'all' (todos
--    los mensajes), 'mentions' (solo @tú/@canal o palabra clave) o 'none' (nada).
--    notify_level es la ÚNICA fuente de verdad de la política de aviso; `muted` (Ola 3)
--    pasa a ser su PROYECCIÓN para el atenuado visual del sidebar: muted ⟺
--    notify_level='none'. SetChannelPref los mantiene sincronizados en la misma fila y
--    ListChannels deriva Muted de notify_level. Backfill: los canales ya silenciados
--    quedan en 'none' para conservar su comportamiento (no notifican + atenuados).
--
-- 2) Preferencias de notificación por usuario (user_notify_prefs): palabras clave que
--    disparan un aviso además de las @menciones, y ventana «No molestar» (DND) en
--    minutos del día [0..1439]; dnd_start=dnd_end = desactivado. El cruce de medianoche
--    (start>end) y la comparación con la hora local los resuelve el cliente. RLS por
--    tenant (mismo patrón deny-by-default que message_acks/read_state).
--
-- ADD COLUMN ... DEFAULT constante lo resuelve Postgres por metadatos, sin reescribir
-- la tabla (barato a escala). channel_members ya tiene su clave por (tenant, channel_id,
-- subject): el JOIN de ListChannels lee notify_level por esa PK, sin índice extra.

ALTER TABLE channel_members ADD COLUMN notify_level text NOT NULL DEFAULT 'all';
ALTER TABLE channel_members ADD CONSTRAINT channel_members_notify_level_valid
    CHECK (notify_level IN ('all', 'mentions', 'none'));

-- Conserva el comportamiento de los canales ya silenciados (Ola 3): silenciar = 'none'.
UPDATE channel_members SET notify_level = 'none' WHERE muted;

-- Preferencias de notificación por usuario. keywords ya en minúsculas (el servicio las
-- sanea); dnd_* en minutos del día. Una fila por (tenant, subject).
CREATE TABLE user_notify_prefs (
    tenant    text   NOT NULL,
    subject   text   NOT NULL,
    keywords  text[] NOT NULL DEFAULT '{}',
    dnd_start int    NOT NULL DEFAULT 0,
    dnd_end   int    NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant, subject),
    CONSTRAINT user_notify_prefs_dnd_range
        CHECK (dnd_start BETWEEN 0 AND 1439 AND dnd_end BETWEEN 0 AND 1439)
);

-- RLS deny-by-default por tenant (mismo patrón que message_acks/read_state).
ALTER TABLE user_notify_prefs ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_notify_prefs FORCE ROW LEVEL SECURITY;
CREATE POLICY user_notify_prefs_tenant_isolation ON user_notify_prefs
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
