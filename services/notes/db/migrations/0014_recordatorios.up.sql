-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- RECORDATORIOS de nota: uno por nota y persona, con hora o de todo el día, y
-- repetición opcional.
--
-- Tabla aparte y no columnas en `notes`: un recordatorio es de QUIEN lo pone, no de
-- la nota (en un cuaderno compartido cada persona pone el suyo), y así no se toca la
-- lista de columnas de las notas.
--
--   · `due_at` es el instante en UTC; `tzid` la zona de quien lo puso, que es en la
--     que se calcula la repetición (las 9:00 de cada lunes son las 9:00 de SU lunes,
--     también cuando cambia la hora).
--   · `rrule` es una RRULE de RFC 5545 sin DTSTART: el DTSTART es `dtstart`, el
--     primer vencimiento, que se queda fijo aunque `due_at` avance. Sin él, un «cada
--     mes» puesto el 31 que cae el 28 de febrero seguiría el 28 para siempre, y un
--     COUNT volvería a contar desde cero en cada vuelta.
--   · `after_days` es la repetición «flexible»: N días DESPUÉS DE COMPLETAR, que no
--     es una RRULE porque no depende del calendario sino de cuándo se hizo.
--   · `fired_at` es cuándo se avisó: el worker solo reclama los que lo tienen a NULL,
--     y por eso avisa una vez.
CREATE TABLE note_reminders (
    note_id    uuid        NOT NULL REFERENCES notes (id) ON DELETE CASCADE,
    tenant     text        NOT NULL,
    owner_sub  text        NOT NULL,
    due_at     timestamptz NOT NULL,
    all_day    boolean     NOT NULL DEFAULT false,
    rrule      text        NOT NULL DEFAULT '',
    after_days integer     NOT NULL DEFAULT 0,
    tzid       text        NOT NULL DEFAULT 'UTC',
    dtstart    timestamptz NOT NULL,
    fired_at   timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (note_id, owner_sub),
    CONSTRAINT note_reminder_after_days CHECK (after_days BETWEEN 0 AND 365),
    CONSTRAINT note_reminder_una_repeticion CHECK (rrule = '' OR after_days = 0),
    CONSTRAINT note_reminder_rrule_len CHECK (length(rrule) <= 200),
    CONSTRAINT note_reminder_tzid_len CHECK (length(tzid) BETWEEN 1 AND 64)
);

-- La cola del worker: lo pendiente por vencimiento, en TODAS las organizaciones.
CREATE INDEX note_reminders_cola ON note_reminders (due_at) WHERE fired_at IS NULL;
-- «Mis recordatorios»: lo pendiente de una persona.
CREATE INDEX note_reminders_persona ON note_reminders (tenant, owner_sub) WHERE fired_at IS NULL;

ALTER TABLE note_reminders ENABLE ROW LEVEL SECURITY;
ALTER TABLE note_reminders FORCE ROW LEVEL SECURITY;
CREATE POLICY note_reminders_tenant_isolation ON note_reminders
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY note_reminders_platform_merge ON note_reminders
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
-- El worker no tiene sesión ni organización: corre bajo `pg.InMaintenanceTx`, como la
-- purga de la papelera (0005). Es la misma variable estrecha, no un BYPASSRLS: una
-- transacción de negocio nunca la fija. Lee `notes` y `notebooks` con las políticas
-- `_mantenimiento` que ya les dio la 0005.
CREATE POLICY note_reminders_mantenimiento ON note_reminders
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
