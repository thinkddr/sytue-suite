-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- DERECHOS RGPD DEL LADO DEL RESPONSABLE (arts. 12 y 15–22; ADR 0054, enmienda
-- del 2026-11-25). Quien responde ante la persona es la organización que trata
-- sus datos, no Sytue: la copia y la baja de la PROPIA cuenta ya existían
-- (portal «Tus datos» y /account/delete), y faltaba lo que necesita quien
-- administra la organización cuando alguien se lo pide a ella:
--
--   * `dsr_requests`, el REGISTRO de solicitudes: quién pidió qué, por qué
--     canal, cuándo llegó, el plazo de un mes (art. 12.3) y su prórroga de dos,
--     cómo se verificó la identidad y cómo se resolvió. Es la prueba de que se
--     atendió (art. 5.2), así que guarda lo justo para eso y nunca lo entregado.
--   * `dsr_request_events`, su historia, solo de añadir: un registro que se
--     puede reescribir no prueba nada.
--   * `users.erasure_request_id`, la SUPRESIÓN FÍSICA: una cuenta dada de baja
--     por una solicitud de supresión no espera los 30 días de la baja normal
--     ni se puede restaurar; el barrido de bajas (BarrerCuentasBorradas) la
--     anonimiza en cuanto caduca el último access token.
--
-- Sin columna `tenant`: Sytue ID no se reparte por tenant ni se fusiona. La RLS es
-- la de 0127: la solicitud se confina por su organización y su historia por la de
-- su solicitud, así que una ruta con el ámbito de una organización no ve ni escribe
-- las de otra aunque al SQL le falte el `org_id`.

CREATE TABLE dsr_requests (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id               uuid        NOT NULL REFERENCES organizations (id),
    -- La persona, si tiene cuenta en la organización. Una solicitud de alguien
    -- de fuera (un cliente, un candidato) solo lleva su correo y su nombre.
    subject_user_id      uuid        REFERENCES users (id),
    subject_email        text        NOT NULL CHECK (length(subject_email) BETWEEN 3 AND 320),
    subject_name         text        NOT NULL DEFAULT '' CHECK (length(subject_name) <= 200),
    kind                 text        NOT NULL CHECK (kind IN
                             ('access', 'rectification', 'erasure', 'restriction', 'portability', 'objection')),
    channel              text        NOT NULL CHECK (channel IN
                             ('email', 'web', 'letter', 'phone', 'in_person', 'other')),
    received_at          timestamptz NOT NULL,
    due_at               timestamptz NOT NULL,
    extended             boolean     NOT NULL DEFAULT false,
    extension_reason     text        NOT NULL DEFAULT '' CHECK (length(extension_reason) <= 1000),
    identity_verified    boolean     NOT NULL DEFAULT false,
    identity_method      text        NOT NULL DEFAULT '' CHECK (length(identity_method) <= 200),
    status               text        NOT NULL DEFAULT 'received' CHECK (status IN
                             ('received', 'in_progress', 'completed', 'rejected')),
    resolution           text        NOT NULL DEFAULT '' CHECK (length(resolution) <= 4000),
    notes                text        NOT NULL DEFAULT '' CHECK (length(notes) <= 4000),
    erasure_started_at   timestamptz,
    erasure_completed_at timestamptz,
    created_by           uuid        REFERENCES users (id),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    closed_at            timestamptz,
    CONSTRAINT dsr_requests_cerrada_check
        CHECK ((status IN ('completed', 'rejected')) = (closed_at IS NOT NULL)),
    CONSTRAINT dsr_requests_supresion_check
        CHECK (erasure_started_at IS NULL OR (kind = 'erasure' AND subject_user_id IS NOT NULL))
);

CREATE INDEX dsr_requests_org_idx ON dsr_requests (org_id, received_at DESC, id DESC);
CREATE INDEX dsr_requests_abiertas_idx ON dsr_requests (due_at) WHERE closed_at IS NULL;

ALTER TABLE dsr_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsr_requests FORCE ROW LEVEL SECURITY;
CREATE POLICY dsr_requests_ambito ON dsr_requests
    USING (id_ambito_org(org_id)) WITH CHECK (id_ambito_org(org_id));

CREATE TABLE dsr_request_events (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    request_id uuid        NOT NULL REFERENCES dsr_requests (id) ON DELETE CASCADE,
    at         timestamptz NOT NULL DEFAULT now(),
    -- NULL = el sistema (el barrido que completa una supresión).
    actor_id   uuid        REFERENCES users (id),
    action     text        NOT NULL CHECK (action ~ '^[a-z_.]{1,64}$'),
    detail     jsonb       NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX dsr_request_events_solicitud_idx ON dsr_request_events (request_id, id);

-- La historia no lleva organización propia: la hereda de su solicitud, que sí está
-- confinada (la subconsulta pasa por la política de dsr_requests). A diferencia de
-- `group_members`, se lee por `request_id` directamente, así que se confina aquí
-- también y no solo por el camino.
ALTER TABLE dsr_request_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsr_request_events FORCE ROW LEVEL SECURITY;
CREATE POLICY dsr_request_events_ambito ON dsr_request_events
    USING (EXISTS (SELECT 1 FROM dsr_requests r WHERE r.id = request_id))
    WITH CHECK (EXISTS (SELECT 1 FROM dsr_requests r WHERE r.id = request_id));

-- Solo de añadir. El borrado en cascada sí pasa: es la retención de la
-- solicitud entera (store.PurgarSolicitudesRGPDVencidas), no una edición.
CREATE FUNCTION dsr_request_events_solo_anadir() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'la historia de una solicitud RGPD no se reescribe'
        USING ERRCODE = 'check_violation';
END $$;

CREATE TRIGGER dsr_request_events_solo_anadir
    BEFORE UPDATE ON dsr_request_events
    FOR EACH ROW EXECUTE FUNCTION dsr_request_events_solo_anadir();

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS erasure_request_id uuid REFERENCES dsr_requests (id);

-- La cola de las supresiones pendientes, junto a la de las bajas propias (0098).
CREATE INDEX IF NOT EXISTS users_supresion_pendiente_idx ON users (closed_at)
    WHERE status = 'deleted' AND erasure_request_id IS NOT NULL AND purged_at IS NULL;
