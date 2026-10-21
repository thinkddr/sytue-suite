-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- PIM para el alcance de PLATAFORMA: la elevación deja de ser permanente.
--
-- `platform_admins` (mig 0031) es una allowlist que, una vez concedida, dura
-- hasta que alguien se acuerda de retirarla. La administración de UNA
-- organización lleva desde la 0020 con elevación temporal —elegible, activas por
-- una ventana, con justificación—, y la superficie que manda sobre TODAS las
-- organizaciones era la única sin ella. Un alcance concedido «para una
-- migración» seguía vivo meses después sin que nada lo dijera.
--
-- El reparto de papeles queda así, igual que en el PIM de organización:
--   · `platform_admins`        = ELEGIBILIDAD. Quién PUEDE elevarse. Por sí sola
--                                ya no autoriza nada.
--   · `platform_activations`   = ACTIVACIÓN. Quién está elevado AHORA, hasta
--                                cuándo y por qué.
--
-- No se reutilizan `role_eligibilities`/`role_activations`: su `org_id` es NOT
-- NULL y es FK a `organizations`, y este alcance no pertenece a ninguna. Hacerlo
-- nullable habría metido un caso especial en cada consulta del PIM de las
-- organizaciones, que funciona, para ahorrar una tabla de seis columnas.
CREATE TABLE platform_activations (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- active | revoked. `expired` NO se persiste: se deriva de expires_at al
    -- leer, igual que en el PIM de organización. Un estado que hay que ir a
    -- escribir con un barrido es un estado que algún día no se escribe, y
    -- entonces una activación caducada sigue autorizando.
    status           text NOT NULL CHECK (status IN ('active', 'revoked')),
    -- Obligatoria y no vacía: el motivo es la mitad del valor de esto. Se pide
    -- en el handler, no aquí, para poder decir POR QUÉ se rechaza.
    justification    text NOT NULL DEFAULT '',
    -- Tope de 8 h: una jornada. Más que eso vuelve a ser un permiso permanente
    -- con otro nombre.
    duration_minutes integer NOT NULL CHECK (duration_minutes BETWEEN 5 AND 480),
    activated_at     timestamptz NOT NULL DEFAULT now(),
    expires_at       timestamptz NOT NULL,
    revoked_at       timestamptz
);

-- La consulta del gate, que corre en CADA petición de la API de plataforma:
-- «¿tiene este usuario una activación viva?». Parcial sobre las activas, que son
-- un puñado frente al histórico completo.
CREATE INDEX platform_activations_vivas_idx ON platform_activations (user_id, expires_at)
    WHERE status = 'active';

-- El histórico por usuario, para el panel y para responder «quién estuvo elevado
-- cuando pasó aquello».
CREATE INDEX platform_activations_user_idx ON platform_activations (user_id, activated_at DESC);
