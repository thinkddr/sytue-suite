-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- UNA APROBACIÓN VALE PARA UN DOCUMENTO, NO PARA UN IMPORTE (NEG-13).
--
-- Hasta aquí la aprobación solo congelaba importe y divisa: con el mismo total se
-- cambiaba el cliente, la cuenta o las líneas después de aprobar, y asentaba. La
-- huella es el SHA-256 de la data del documento al pedirla; asentar o emitir
-- exige que siga siendo la misma. Las de antes quedan con '' y se piden otra vez.
ALTER TABLE aprobacion ADD COLUMN huella text NOT NULL DEFAULT '';
