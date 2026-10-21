ALTER TABLE assets
    DROP CONSTRAINT IF EXISTS assets_lugar_en_rango,
    DROP CONSTRAINT IF EXISTS assets_lugar_completo,
    DROP COLUMN IF EXISTS longitude,
    DROP COLUMN IF EXISTS latitude;
