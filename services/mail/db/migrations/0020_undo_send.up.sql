-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- VENTANA DE DESHACER EL ENVÍO, por cuenta y en el servidor.
--
-- Era una constante de 10 s en el cliente web: quien escribe deprisa y se arrepiente
-- tarde no podía alargarla, y quien no la quiere tan larga tampoco acortarla. Va en
-- la CUENTA (como la firma, 0014) y no en el navegador: la ventana es una costumbre
-- de quien escribe, no del equipo desde el que escribe.
--
-- Columna de `accounts` por lo mismo que la firma: un escalar con la vida de la
-- cuenta. `accounts` ya va con ENABLE y FORCE ROW LEVEL SECURITY desde 0001 y las
-- políticas son de tabla, así que la columna queda cubierta sin declarar nada.
--
-- Solo los cuatro valores que ofrece la interfaz (los de Gmail). El CHECK es la
-- última línea: el servicio ya rechaza lo demás con un 400, pero así un UPDATE a
-- mano tampoco deja una ventana que ningún cliente sabría pintar. El 10 por defecto
-- es la ventana que había: nadie nota el cambio hasta que la toca.

ALTER TABLE accounts
  ADD COLUMN undo_send_seconds smallint NOT NULL DEFAULT 10
  CONSTRAINT accounts_undo_send_seconds_check CHECK (undo_send_seconds IN (5, 10, 20, 30));
