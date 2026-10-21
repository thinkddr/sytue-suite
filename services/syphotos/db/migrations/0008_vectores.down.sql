ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_vector_con_modelo;
DROP INDEX IF EXISTS assets_sin_vector;
ALTER TABLE assets
    DROP COLUMN IF EXISTS embedding_model,
    DROP COLUMN IF EXISTS embedding;
