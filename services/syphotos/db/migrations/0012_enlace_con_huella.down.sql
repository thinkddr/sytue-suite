-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- sytue:destructive-ok la huella no devuelve el token: al bajar, los álbumes quedan sin enlace y se comparten otra vez.
DROP POLICY IF EXISTS albums_enlace_publico ON albums;
CREATE POLICY albums_enlace_publico ON albums
    FOR SELECT
    USING (
        current_setting('app.public_link', true) = 'on'
        AND length(public_token) > 0
    );
DROP INDEX IF EXISTS albums_public_token_hash;
ALTER TABLE albums DROP COLUMN public_token_hash;
