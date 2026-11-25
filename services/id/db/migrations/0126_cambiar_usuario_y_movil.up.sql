-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Cambiar el @usuario y el móvil desde Mi cuenta (ADR 0040 y 0041).
--
-- EL MÓVIL NUEVO SE VERIFICA COMO EN EL ALTA: un código de seis dígitos por SMS, diez
-- minutos, cinco intentos y reenvío tras un minuto. Uno pendiente por persona: pedir
-- otro sustituye al anterior. El número no se fija hasta confirmar el código, así que
-- una sesión robada no puede ponerle a nadie un móvil que no controla. Es también como
-- las cuentas anteriores a la 0066, que no tienen móvil, añaden uno.
CREATE TABLE phone_change_codes (
    user_id    uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    phone      text NOT NULL,
    code_hash  text NOT NULL,
    attempts   int NOT NULL DEFAULT 0,
    sent_at    timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);
CREATE INDEX phone_change_codes_expires_idx ON phone_change_codes (expires_at);

-- EL @USUARIO QUE SE DEJA NO LO COGE OTRA PERSONA EN 30 DÍAS. Quien te escribía como
-- @ana seguiría haciéndolo, y si otra persona se lo quedara al momento recibiría lo
-- que iba para ti. Durante ese tiempo solo lo puede recuperar quien lo dejó.
CREATE TABLE usernames_retirados (
    username   text PRIMARY KEY,               -- en minúsculas
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    hasta      timestamptz NOT NULL
);
CREATE INDEX usernames_retirados_hasta_idx ON usernames_retirados (hasta);

-- Cuándo se cambió por última vez: uno cada 30 días, que es lo que dura la reserva
-- del anterior. Sin esto se podrían ir dejando reservados nombres sin fin.
ALTER TABLE users ADD COLUMN username_changed_at timestamptz;
