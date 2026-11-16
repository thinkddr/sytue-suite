-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- El cumpleaños de relleno de Apple que la subida pasó a `birthday_no_year` no vuelve a
-- `birthday`: sigue en la tarjeta (card_objects.data), que es la fuente de verdad.
ALTER TABLE contacts DROP COLUMN IF EXISTS birthday_no_year;
