-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

CREATE OR REPLACE FUNCTION contacts_search_update() RETURNS trigger AS $$
BEGIN
    NEW.search :=
        setweight(to_tsvector('simple', coalesce(NEW.formatted_name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(NEW.family_name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(NEW.given_name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(NEW.nickname, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(NEW.organization, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(NEW.title, '')), 'C') ||
        setweight(to_tsvector('simple', coalesce(NEW.note, '')), 'D');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
ALTER TABLE contacts NO FORCE ROW LEVEL SECURITY;
UPDATE contacts SET search = NULL;
ALTER TABLE contacts FORCE ROW LEVEL SECURITY;
ALTER TABLE contacts ALTER COLUMN search_text
    SET EXPRESSION AS (lower(
        formatted_name || ' ' || family_name || ' ' || given_name || ' ' ||
        nickname || ' ' || organization || ' ' || title
    ));

-- plegar() y unaccent se quedan: la versión anterior no los usa y retirarlos
-- obligaría a soltar antes cualquier índice que dependa de ellos.
