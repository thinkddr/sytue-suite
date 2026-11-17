-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- TIPOS CON FORMATO: URL, correo, teléfono, moneda y valoración. Se guardan en `data`
-- como el texto o el número que son (la forma la exige el servicio, ver
-- internal/domain/formatos.go); la moneda lleva su código en `options.currency`. Solo
-- se amplía el CHECK: ningún dato existente cambia.
ALTER TABLE base_fields DROP CONSTRAINT base_fields_kind_check;
ALTER TABLE base_fields ADD CONSTRAINT base_fields_kind_check
    CHECK (kind IN ('text', 'number', 'date', 'checkbox', 'select', 'multiselect', 'link', 'formula', 'attachment',
                    'url', 'email', 'phone', 'currency', 'rating'));
