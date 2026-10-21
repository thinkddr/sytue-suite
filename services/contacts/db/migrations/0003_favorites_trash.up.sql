-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Favoritos + Papelera (paridad Google Contacts «destacados» + «papelera»).
--
-- FAVORITOS: en Contacts cada contacto pertenece a UNA libreta de UN dueño (no
-- hay compartición por-usuario como en Drive), así que «destacado» es una
-- propiedad del propio contacto, no una tabla de marcas por-usuario: un flag
-- `is_favorite` en la fila basta y es correcto (el dueño de la libreta es el
-- único que ve el contacto). El borrado suave lo oculta del filtro (WHERE
-- deleted_at IS NULL) y restaurar lo revive; purgar borra la fila y con ella el
-- flag (sin marcas colgantes). Reusa el patrón «oculta/revive» de Drive.
ALTER TABLE contacts ADD COLUMN is_favorite boolean NOT NULL DEFAULT false;

-- Filtro «Favoritos» del dueño (CONVENTIONS §0.8: índice para TODO WHERE). El
-- filtro por owner_sub lo resuelve el JOIN a address_books (abook_owner_idx).
CREATE INDEX contacts_favorite_idx
    ON contacts (tenant, address_book_id)
    WHERE is_favorite AND deleted_at IS NULL;

-- PAPELERA: el borrado suave (deleted_at) YA existía; faltaba poder listarla.
-- Ningún índice previo cubría las filas BORRADAS (todos son WHERE deleted_at IS
-- NULL), así que listar la papelera del dueño sería un seq-scan. Añadimos un
-- índice keyset por reciente-borrado-primero para el recorrido sin OFFSET ni
-- ordenación en memoria (keyset (deleted_at DESC, id DESC), sin N+1).
CREATE INDEX contacts_trash_idx
    ON contacts (tenant, deleted_at DESC, id DESC)
    WHERE deleted_at IS NOT NULL;
