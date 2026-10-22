-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Dos personas para lo que no se puede deshacer.
--
-- Suspender una organización corta el acceso de toda una empresa, y fusionar es
-- literalmente irreversible: los datos de los nueve servicios se reetiquetan con
-- el tenant del destino y el origen queda en un estado terminal. Hasta ahora
-- bastaba UN admin global elevado, y las defensas eran del tipo «escribe el
-- identificador para confirmar»: protegen del despiste, no de la cuenta
-- comprometida ni de la decisión tomada a solas de madrugada.
--
-- El PIM (0043) ya hizo temporal el alcance; esto hace COLEGIADO lo irreversible.
-- La petición y la aprobación son de personas DISTINTAS, y quien aprueba es
-- quien ejecuta: no queda una solicitud aprobada esperando a que alguien pulse.
--
-- `payload` es jsonb y no columnas por acción: son dos operaciones con
-- parámetros distintos —una organización y un motivo; dos organizaciones— y
-- media docena de columnas nullables donde cada fila usa la mitad describe peor
-- lo que hay. Lo que se ejecuta se valida al aprobar, contra el estado de ese
-- momento y no el de cuando se pidió.
--
-- La caducidad NO se persiste: se deriva de expires_at al leer, como en la 0043.
-- Un estado que hay que ir a escribir con un barrido es un estado que algún día
-- no se escribe, y entonces una solicitud de hace tres meses sigue viva.
CREATE TABLE platform_approvals (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    action       text NOT NULL CHECK (action IN ('org.suspend', 'org.merge')),
    payload      jsonb NOT NULL,
    -- Obligatorio y no vacío (lo exige el handler, para poder decir por qué se
    -- rechaza): quien aprueba necesita saber qué está aprobando, y quien lo mire
    -- dentro de un año no tendrá a nadie a quien preguntar.
    reason       text NOT NULL DEFAULT '',
    requested_by uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- pending | executed | rejected. `expired` se deriva.
    status       text NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'executed', 'rejected')),
    decided_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    decided_at   timestamptz,
    -- Qué pasó al ejecutar (id de la fusión creada, error…). Para que el panel
    -- no tenga que cruzar tablas para contar el final de la historia.
    result       text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL
);

-- La consulta del panel y del gate: solicitudes vivas. Parcial, porque las
-- pendientes son siempre un puñado frente al histórico.
CREATE INDEX platform_approvals_pendientes_idx
    ON platform_approvals (expires_at)
    WHERE status = 'pending';

-- El histórico completo, en orden inverso.
CREATE INDEX platform_approvals_created_idx ON platform_approvals (created_at DESC);
