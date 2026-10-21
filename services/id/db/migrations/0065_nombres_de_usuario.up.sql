-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El @usuario: el nombre con el que alguien de OTRA organización te encuentra en
-- Sytue One para abrirte un mensaje, un canal o una reunión (ADR 0040).
--
-- Global y único sin distinguir mayúsculas, como las direcciones: @ana es
-- ana@sytue.eu, y en una cuenta personal esa es literalmente su dirección.
--
-- Expand puro. La columna nace ANULABLE a propósito: durante el despliegue
-- conviven pods del binario anterior, que dan de alta sin mandarla, y un NOT NULL
-- rechazaría esas altas. Quien llegue sin @usuario —todo el que existía y lo que
-- entre por un pod viejo— lo recibe del barrido de Sytue ID
-- (store.AsignarUsernamesPendientes), que usa la MISMA regla que el alta en vez
-- de una copia en SQL. El NOT NULL es el paso de contract de la versión siguiente.

ALTER TABLE users ADD COLUMN username text;

-- text_pattern_ops y no el operador por defecto: además de la unicidad, el alta
-- busca los ocupados por PREFIJO (ana.perez, ana.perez2…) y con una intercalación
-- que no sea C ese LIKE no puede usar un índice corriente.
CREATE UNIQUE INDEX users_username_idx ON users (lower(username) text_pattern_ops);

-- La cola del barrido. Parcial: en régimen normal está vacía.
CREATE INDEX users_username_pendiente_idx ON users (created_at) WHERE username IS NULL;
