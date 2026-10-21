-- sytue:destructive-ok deshace la migración 0030 en el mismo despliegue; la tabla
-- es de estado efímero (quién espera en la puerta ahora mismo) y ninguna versión
-- anterior la leyó nunca.
DROP TABLE IF EXISTS room_admissions;
