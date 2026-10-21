-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- EDICIÓN de un álbum compartido con una persona (decisión del dueño, 2026-09-28):
-- quien la tiene añade fotos, quita cualquiera y lo renombra. Lo que añade se COPIA al
-- Drive de la dueña (Drive: /internal/photos/copies), así que el álbum no depende de
-- que esa persona conserve sus originales; y al retirarle el permiso, lo copiado se
-- queda con la dueña.
ALTER TABLE album_personas ADD COLUMN puede_editar boolean NOT NULL DEFAULT false;
