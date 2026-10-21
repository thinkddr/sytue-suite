-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- BAJA Y RESTAURACIÓN de una cuenta desde Sytue Admin.
--
-- Lo que había: `status='deleted'` lo escribía SOLO el desaprovisionamiento
-- SCIM, y la única puerta desde la consola era suspender. Así que una
-- organización sin IdP corporativo no podía dar de baja a nadie —la cuenta
-- quedaba suspendida para siempre— y, cuando la baja sí ocurría, era una puerta
-- de un solo sentido: `SetUserStatus` se niega a tocar una fila `deleted` (la
-- verja anti-resurrección de users.go), así que un error de dedo o una baja
-- revertida a los dos días acababa en un ticket a Sytue.
--
-- Estas tres columnas son lo que faltaba para que la baja sea REVERSIBLE con
-- criterio y no un `UPDATE` a mano: cuándo se dio, quién la dio y por qué.
-- Sin `closed_at` no hay ventana de recuperación que valga —no se sabe si la
-- baja fue ayer o hace dos años—, y una restauración sin ventana es que el
-- borrado no significaba nada.
--
-- La verja anti-resurrección se QUEDA donde está: `SetUserStatus` sigue sin
-- poder revivir a nadie. La restauración es un camino propio, explícito y
-- auditado (`RestoreUser`), no un efecto lateral de reactivar.
--
-- El purgado de los datos al vencer la ventana NO entra aquí: esto guarda
-- cuándo vence, y quien barra lo tendrá escrito. Mientras tanto la cuenta sigue
-- sin poder entrar (todos los caminos de login filtran por status).

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS closed_at    timestamptz,
    ADD COLUMN IF NOT EXISTS closed_by    uuid REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS close_reason text NOT NULL DEFAULT '';

-- El listado de bajas recuperables es «las de esta organización, por fecha»:
-- parcial, porque las cuentas dadas de baja son una minoría diminuta de la tabla.
CREATE INDEX IF NOT EXISTS users_closed_at_idx ON users (closed_at DESC)
    WHERE status = 'deleted' AND closed_at IS NOT NULL;
