-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- `birthday_no_year`: un cumpleaños SIN año, «--MM-DD» («--0215» en vCard 4, o el año
-- de relleno que Contactos de Apple marca con X-APPLE-OMIT-YEAR).
--
-- POR QUÉ. `birthday` es `date` y una fecha sin año no cabe: la proyección lo
-- descartaba y la app nativa, que lee por la API REST, no lo veía ni en la ficha ni en
-- «Próximos cumpleaños»; la web sí, porque lee la vCard. Va en una columna APARTE y no
-- con un año ficticio en `birthday`: quien lee la API sin conocerla seguiría
-- calculando una edad de dos mil años. Vacía ('') y no NULL, como el resto de textos
-- de la proyección. Excluye a `birthday`: el servicio solo rellena una de las dos.
ALTER TABLE contacts
    ADD COLUMN birthday_no_year text NOT NULL DEFAULT ''
        CONSTRAINT contacts_birthday_no_year_valido
        CHECK (birthday_no_year = '' OR birthday_no_year ~ '^--(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$');

-- Los contactos que ya estaban: se leen de la tarjeta guardada con la misma regla que
-- `leerCumple` (internal/domain/cumple.go). Bajo FORCE ROW LEVEL SECURITY un UPDATE
-- sin organización no ve ninguna fila, así que se suelta el FORCE solo dentro de esta
-- transacción (la migración entera es una) y se vuelve a poner antes de terminar,
-- como en 0006.
ALTER TABLE contacts NO FORCE ROW LEVEL SECURITY;
ALTER TABLE card_objects NO FORCE ROW LEVEL SECURITY;

WITH bday AS (
    -- El primer BDAY (con o sin grupo, `item1.BDAY`) de la tarjeta desplegada
    -- (RFC 6350 §3.2): [1] sus parámetros, [2] su valor.
    SELECT c.tenant, c.id, c.birthday,
           regexp_match(regexp_replace(o.data, '\r?\n[ \t]', '', 'g'),
                        '(?:^|\n)(?:[^\n:;]*\.)?BDAY((?:;[^\n:]*)?):[ \t]*([^\r\n]*)', 'i') AS m
    FROM contacts c
    JOIN card_objects o ON o.tenant = c.tenant AND o.id = c.object_id
), sin_ano AS (
    SELECT tenant, id,
           CASE
               -- «--0215» / «--02-15».
               WHEN birthday IS NULL AND m[2] ~ '^--[0-9]{2}-?[0-9]{2}[ \t]*$'
                   THEN '--' || substr(m[2], 3, 2) || '-' || substr(replace(m[2], '-', ''), 3, 2)
               -- El año de relleno de Apple: la proyección vieja lo tomaba por real.
               WHEN birthday IS NOT NULL
                    AND substring(m[1] from '(?i);X-APPLE-OMIT-YEAR=([0-9]{4})')::int = extract(year FROM birthday)
                   THEN to_char(birthday, '"--"MM-DD')
           END AS v
    FROM bday
    WHERE m IS NOT NULL
)
UPDATE contacts c
SET birthday_no_year = s.v, birthday = NULL
FROM sin_ano s
WHERE c.tenant = s.tenant AND c.id = s.id
  -- Un día que exista (en un año bisiesto, para que el 29 de febrero valga): un
  -- «--0231» o un «--1301» se quedan fuera, como en `NormalizeBirthdayNoYear`. Con
  -- CASE, que garantiza el orden: `make_date` con el mes 13 sería un error.
  AND CASE WHEN s.v ~ '^--(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$'
           THEN extract(month FROM make_date(2000, substr(s.v, 3, 2)::int, 1) + (substr(s.v, 6, 2)::int - 1))
                = substr(s.v, 3, 2)::int
           ELSE false
      END;

ALTER TABLE card_objects FORCE ROW LEVEL SECURITY;
ALTER TABLE contacts FORCE ROW LEVEL SECURITY;
