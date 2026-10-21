-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Preferencias de notificación POR USUARIO, por origen y por tipo.
--
-- Hasta ahora las notificaciones unificadas llegaban todas: con seis apps publicando,
-- el centro de notificaciones se convierte en ruido y la gente deja de mirarlo — que es
-- la forma más cara de perder un aviso importante.
--
-- El modelo es una lista de EXCEPCIONES, no una matriz completa: solo se guarda lo que
-- el usuario cambia, y lo no dicho llega (opt-out). Así una app nueva no queda muda
-- esperando a que alguien la habilite, y la tabla no crece con filas «sí, como siempre».
--
-- `kind` vacío = la regla aplica a TODO el origen; con valor, solo a ese tipo de aviso.
-- La regla más específica (origen+tipo) gana sobre la del origen entero.
CREATE TABLE notification_prefs (
    tenant     text        NOT NULL,
    subject    text        NOT NULL,
    source     text        NOT NULL,
    -- kind: tipo de aviso dentro del origen ('' = todos los del origen).
    kind       text        NOT NULL DEFAULT '',
    -- muted: true = no entregar. Es lo único que se guarda: una preferencia que no
    -- silencia nada no hace falta persistirla.
    muted      boolean     NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, subject, source, kind)
);

ALTER TABLE notification_prefs ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_prefs FORCE ROW LEVEL SECURITY;

CREATE POLICY notification_prefs_tenant_isolation ON notification_prefs
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
