-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Buscar un usuario en el directorio de una organización, DE VERDAD.
--
-- El listado admin pagina por keyset de 50 en 50 (0007) y la consola filtraba
-- sobre lo que tuviera cargado: en una organización de veinte mil personas, el
-- buscador decía «sin resultados» cuando lo que pasaba era «sin resultados
-- entre las cincuenta primeras». No fallaba nada, y ese es el problema.
--
-- pg_trgm (contrib estándar de PostgreSQL, licencia permisiva; ya en uso en
-- contacts, drive, docs, mail y calendar) sirve el ILIKE '%q%' por índice, que
-- es lo que hace falta: se busca por trozo de correo o por apellido, no por
-- prefijo. Columna GENERATED STORED y no un trigger: se calcula sola para las
-- filas de hoy y las de mañana. La concatenación es con `||` —una columna
-- generada exige expresión IMMUTABLE y concat_ws solo es STABLE— y es segura
-- porque `email` es NOT NULL y `name` es NOT NULL DEFAULT ''.
--
-- Sin `WHERE status <> 'deleted'`: el directorio admin también lista las
-- cuentas suspendidas, y buscar a alguien para reactivarlo es justo uno de los
-- motivos por los que se busca.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

ALTER TABLE users
    ADD COLUMN search_text text
    GENERATED ALWAYS AS (lower(email || ' ' || name)) STORED;

CREATE INDEX users_search_trgm_idx ON users USING gin (search_text gin_trgm_ops);
