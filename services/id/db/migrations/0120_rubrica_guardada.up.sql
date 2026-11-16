-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- La rúbrica dibujada que la persona pidió recordar al firmar en CertiPDF
-- («Recordar mi firma»), para no volver a dibujarla en cada documento.
--
-- Es de la CUENTA y no del dispositivo: se borra desde Mi cuenta (otro origen que
-- CertiPDF, así que no podía vivir en el navegador) y desaparece con la cuenta
-- (ON DELETE CASCADE). Los trazos van CIFRADOS con la KEK del servicio
-- (keycrypto, "encv1:…"): una firma manuscrita es un dato personal que permite
-- suplantar a alguien, y no debe leerse en claro en una copia de la base.
-- Tabla 1:1 aparte, como user_avatars: ninguna lectura de `users` la necesita.
CREATE TABLE user_rubricas (
    user_id         uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    trazos_cifrados text NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now()
);
