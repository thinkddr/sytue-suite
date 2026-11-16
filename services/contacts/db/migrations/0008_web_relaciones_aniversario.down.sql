-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Los sitios web, las relaciones y el aniversario siguen en la tarjeta
-- (card_objects.data), que es la fuente de verdad: bajar solo quita la proyección.
ALTER TABLE contacts
    DROP COLUMN IF EXISTS urls,
    DROP COLUMN IF EXISTS related,
    DROP COLUMN IF EXISTS anniversary;
