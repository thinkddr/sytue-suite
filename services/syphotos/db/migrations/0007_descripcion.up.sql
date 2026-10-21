-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- QUÉ SALE EN LA FOTO: una descripción y unas etiquetas.
--
-- Las escribe Sytue AI mirando la miniatura, a petición de su dueña. Sirven
-- para dos cosas distintas y las dos importan:
--
--   · El TEXTO ALTERNATIVO. Hasta ahora el `alt` de cada foto era el nombre del
--     fichero —«IMG_20260714_193045.jpg»—, que para un lector de pantalla no
--     dice absolutamente nada.
--   · La BÚSQUEDA POR CONTENIDO. Buscar «playa» encontraba las fotos que
--     alguien se acordó de llamar «playa», no las que tienen una playa.
--
-- Van en `assets` y no en una tabla aparte porque son atributos de la foto: hay
-- una descripción por foto y las etiquetas no llevan datos propios. El día que
-- una etiqueta necesite color, o contarse, o compartirse, será una tabla.
ALTER TABLE assets
    ADD COLUMN description text   NOT NULL DEFAULT '',
    ADD COLUMN tags        text[] NOT NULL DEFAULT '{}';

-- Ni índice de texto completo ni GIN, todavía.
--
-- La búsqueda de la línea de tiempo es un ILIKE sobre la línea de UNA persona,
-- que es lo que ya hacía con el nombre; añadirle dos columnas no cambia el
-- orden de magnitud. Cuando una biblioteca de cientos de miles lo note, el
-- sitio es un `tsvector` generado sobre nombre + descripción + etiquetas con su
-- GIN, y entonces la consulta cambia de forma. Hacerlo ahora sería mantener un
-- índice que nadie ha medido.
