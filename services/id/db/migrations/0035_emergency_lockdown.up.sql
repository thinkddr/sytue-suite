-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- BLOQUEO DE EMERGENCIA — la mitad que le faltaba al interruptor.
--
-- `id revoke-all` invalida en el acto TODO lo emitido (rota el material de firma
-- y retira el anterior sin ventana de gracia), y rotar el `cookie-secret` hace
-- lo propio con las cookies de las diez superficies. Pero ninguna de las dos
-- cosas impide EMITIR de nuevo: quien tenga la credencial —el atacante incluido,
-- que es el supuesto— vuelve a entrar un segundo después y todo el corte se
-- deshace solo. Cerrar de verdad exige una marca que se consulte al emitir.
--
-- Por qué en la BASE y no en una variable de entorno o un flag del proceso:
-- levantarlo y bajarlo tiene que ser inmediato y global, sin reiniciar nada y
-- sin depender de que las réplicas converjan. Es el mismo criterio de
-- `platform_admins` (security.md §4.9): las decisiones que deben poder
-- REVOCARSE en el acto se consultan, no se cachean en un token.
--
-- Fila única: el bloqueo es del sistema entero, no por organización. Un bloqueo
-- por tenant sería otra función, con su propia API y su propia auditoría.
CREATE TABLE emergency_lockdown (
    -- Truco de tabla singleton: la PK solo admite `true`, así que no puede
    -- haber dos filas discrepando sobre si el sistema está cerrado.
    id            boolean PRIMARY KEY DEFAULT true CHECK (id),
    active        boolean NOT NULL DEFAULT false,
    -- Motivo obligatorio al activar, por lo mismo que en la administración de
    -- plataforma: un corte total sin motivo escrito es indistinguible de un
    -- error seis meses después.
    reason        text NOT NULL DEFAULT '',
    -- Quién lo movió por última vez. Sin FK a users: el bloqueo lo levanta un
    -- operador desde el binario, que es justo el escenario en el que la consola
    -- (y puede que la propia cuenta) es lo que ha fallado.
    actor         text NOT NULL DEFAULT '',
    changed_at    timestamptz NOT NULL DEFAULT now()
);

-- La fila existe desde el principio y en reposo: así la consulta del camino de
-- emisión es un SELECT que siempre encuentra algo, y «no hay fila» no puede
-- confundirse con «no está bloqueado».
INSERT INTO emergency_lockdown (id, active, reason, actor)
VALUES (true, false, '', '')
ON CONFLICT (id) DO NOTHING;
