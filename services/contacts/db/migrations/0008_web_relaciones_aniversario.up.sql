-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sitios web (URL), personas relacionadas (RELATED) y aniversario (ANNIVERSARY).
--
-- POR QUÉ. La web los lee de la vCard; la app nativa va por la API REST, que no los
-- proyectaba: no los veía en la ficha y, como el PUT sustituye el contacto entero, no
-- podía reenviarlos. `urls` y `related` son listas pequeñas que siempre se leen y se
-- escriben enteras con la fila, así que van en jsonb y no en tablas aparte como los
-- correos (que sí se buscan). Vacías ('[]') y no NULL, como el resto de la proyección.
ALTER TABLE contacts
    ADD COLUMN urls jsonb NOT NULL DEFAULT '[]'
        CONSTRAINT contacts_urls_lista CHECK (jsonb_typeof(urls) = 'array'),
    ADD COLUMN related jsonb NOT NULL DEFAULT '[]'
        CONSTRAINT contacts_related_lista CHECK (jsonb_typeof(related) = 'array'),
    ADD COLUMN anniversary date;

-- Los contactos que ya estaban: se leen de la tarjeta guardada con la misma regla que
-- el servicio (internal/domain/enlaces.go). Como en 0007, se suelta el FORCE ROW LEVEL
-- SECURITY solo dentro de esta transacción y se vuelve a poner antes de terminar.
ALTER TABLE contacts NO FORCE ROW LEVEL SECURITY;
ALTER TABLE card_objects NO FORCE ROW LEVEL SECURITY;

CREATE TEMPORARY TABLE tarjetas_0008 ON COMMIT DROP AS
SELECT c.tenant, c.id, regexp_replace(o.data, '\r?\n[ \t]', '', 'g') AS d
FROM contacts c
JOIN card_objects o ON o.tenant = c.tenant AND o.id = c.object_id;

-- Cada línea de la propiedad (con o sin grupo, `item1.URL`), en orden: [1] sus
-- parámetros, [2] su valor sin desescapar (go-vcard desescapa «\\» y «\,»).
WITH lineas AS (
    SELECT t.tenant, t.id, r.n, r.m[1] AS params,
           btrim(replace(replace(r.m[2], '\,', ','), '\\', '\'), E' \t') AS v
    FROM tarjetas_0008 t,
         regexp_matches(t.d, '(?:^|\n)(?:[^\n:;]*\.)?URL((?:;[^\n:]*)?):([^\r\n]*)', 'gi')
             WITH ORDINALITY AS r(m, n)
), proyectables AS (
    -- Solo una dirección http(s) completa, con su máquina, sin usuario y sin espacios
    -- (NormalizeURL): lo demás queda oculto en la tarjeta.
    SELECT tenant, id, n, v,
           CASE lower(substring(params FROM '(?i)TYPE="?[^;:"]*?\m(home|work)\M'))
               WHEN 'home' THEN 'home' WHEN 'work' THEN 'work' ELSE 'other'
           END AS tipo
    FROM lineas
    WHERE v ~* '^https?://([^\s/?#:@\[\]]+|\[[0-9a-f:.]+\])(:[0-9]*)?([/?#]|$)'
      AND v !~ '\s' AND octet_length(v) <= 2048
)
UPDATE contacts c
SET urls = s.lista
FROM (SELECT tenant, id, jsonb_agg(jsonb_build_object('url', v, 'type', tipo) ORDER BY n) AS lista
      FROM proyectables GROUP BY tenant, id) s
WHERE c.tenant = s.tenant AND c.id = s.id;

WITH lineas AS (
    SELECT t.tenant, t.id, r.n, r.m[1] AS params, r.m[2] AS crudo,
           btrim(replace(replace(r.m[2], '\,', ','), '\\', '\'), E' \t') AS v
    FROM tarjetas_0008 t,
         regexp_matches(t.d, '(?:^|\n)(?:[^\n:;]*\.)?RELATED((?:;[^\n:]*)?):([^\r\n]*)', 'gi')
             WITH ORDINALITY AS r(m, n)
), proyectables AS (
    -- Solo las de texto (un nombre): VALUE=text, o un valor que no es una URI.
    SELECT tenant, id, n, v,
           lower(substring(params FROM '(?i)TYPE="?[^;:"]*?\m(contact|acquaintance|friend|met|co-worker|colleague|co-resident|neighbor|child|parent|sibling|spouse|kin|muse|crush|date|sweetheart|me|agent|emergency)\M')) AS tipo
    FROM lineas
    WHERE v <> '' AND crudo !~ '\\[nN]' AND octet_length(v) <= 256
      AND (params ~* ';VALUE="?text' OR v !~ '^[A-Za-z][A-Za-z0-9+.-]*:\S*$')
)
UPDATE contacts c
SET related = s.lista
FROM (SELECT tenant, id,
             jsonb_agg(CASE WHEN tipo IS NULL THEN jsonb_build_object('name', v)
                            ELSE jsonb_build_object('name', v, 'type', tipo) END ORDER BY n) AS lista
      FROM proyectables GROUP BY tenant, id) s
WHERE c.tenant = s.tenant AND c.id = s.id;

-- El primer ANNIVERSARY, si es una fecha completa («2010-06-12» o «20100612»); uno
-- parcial («--0612») se queda oculto.
WITH primero AS (
    SELECT t.tenant, t.id,
           regexp_match(t.d, '(?:^|\n)(?:[^\n:;]*\.)?ANNIVERSARY(?:;[^\n:]*)?:[ \t]*([0-9]{4})(-?)([0-9]{2})\2([0-9]{2})[ \t]*(?:\r|\n|$)', 'i') AS m
    FROM tarjetas_0008 t
)
UPDATE contacts c
SET anniversary = make_date(p.m[1]::int, p.m[3]::int, p.m[4]::int)
FROM primero p
WHERE c.tenant = p.tenant AND c.id = p.id AND p.m IS NOT NULL
  -- Un día que exista: con CASE, que garantiza el orden (make_date con el mes 13 o el
  -- año 0 sería un error y pararía la migración entera).
  AND CASE WHEN p.m[1]::int >= 1 AND p.m[3]::int BETWEEN 1 AND 12 AND p.m[4]::int BETWEEN 1 AND 31
           THEN extract(month FROM make_date(p.m[1]::int, p.m[3]::int, 1) + (p.m[4]::int - 1)) = p.m[3]::int
           ELSE false
      END;

ALTER TABLE card_objects FORCE ROW LEVEL SECURITY;
ALTER TABLE contacts FORCE ROW LEVEL SECURITY;
