DROP POLICY IF EXISTS albums_enlace_publico ON albums;
DROP INDEX IF EXISTS albums_public_token;
ALTER TABLE albums DROP COLUMN IF EXISTS public_token;
