-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Miniatura de la portada: la primera página, hoja o diapositiva del documento en un PNG
-- pequeño (domain/miniatura.go), para que la lista enseñe qué hay dentro y no solo de
-- qué tipo es.
--
-- El PNG va a Object Storage (docs/<id>/miniatura/<seq>.png, bajo el prefijo que la
-- papelera borra entero); aquí solo su clave y la secuencia del documento con la que se
-- dibujó. `thumbnail_seq` es lo que viaja en la lista: hace de versión en la URL de la
-- imagen, que así se cachea para siempre y cambia cuando cambia el documento. 0 = sin
-- miniatura todavía, y la web pinta el glifo del tipo.
--
-- Guardarla NO toca `updated_at`: dibujar la miniatura no es editar el documento, y la
-- lista se ordena por esa fecha.
ALTER TABLE documents ADD COLUMN IF NOT EXISTS thumbnail_key text;
ALTER TABLE documents ADD COLUMN IF NOT EXISTS thumbnail_seq bigint NOT NULL DEFAULT 0;
