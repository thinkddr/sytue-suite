-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Dos columnas para que la consola de plataforma pueda poner propietario a una
-- organización y atender una cuenta de cualquier organización sin inventar
-- mecanismos nuevos: los dos cuelgan de los que ya había.
--
-- 1. `users.password_must_change`: la contraseña TEMPORAL que fija un operador
--    de plataforma vale para entrar UNA vez y obliga a cambiarla. El login, al
--    verla, no abre sesión: manda al formulario de restablecimiento de siempre
--    (/reset-password) con un token recién emitido. `SetPassword` la baja, así
--    que cualquier vía por la que la persona elija contraseña la apaga.
--
-- 2. `org_invitations.invitee_name`: «crear propietario» es una invitación con
--    rol owner, y la plataforma la manda con el nombre de la persona. Al aceptar
--    se le pone ese nombre a la cuenta nueva en vez de la parte local del correo.
--    Vacío = como hasta ahora.
ALTER TABLE users ADD COLUMN password_must_change boolean NOT NULL DEFAULT false;

ALTER TABLE org_invitations ADD COLUMN invitee_name text NOT NULL DEFAULT '';
