-- sytue:destructive-ok deshace la migración 0029 en el mismo despliegue; ninguna
-- versión anterior leyó nunca esta columna.
ALTER TABLE rooms DROP COLUMN IF EXISTS locked;
