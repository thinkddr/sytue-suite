-- Revertir el contract devuelve la COLUMNA, nunca los tokens.
--
-- Y hay que decirlo con todas las letras: los secretos borrados no vuelven. Esta
-- bajada recrea `token` vacía para que el esquema encaje con el de antes, pero
-- los enlaces creados antes de 0018 NO recuperan su cadena en claro —solo existe
-- ya como hash, que es irreversible por definición—.
--
-- Eso no rompe nada del servicio: desde 0018 resuelve por `token_hash`, así que
-- los enlaces siguen funcionando igual con la columna vacía. Lo que no se puede
-- es volver a una versión ANTERIOR a 0018, que leía el token en claro: para esa
-- no hay vuelta atrás, y por eso el contract no se hace hasta que esa versión
-- deja de estar viva.
--
ALTER TABLE shares ADD COLUMN IF NOT EXISTS token text;

-- Y se devuelve el CHECK a la forma de 0019 —la que acepta CUALQUIERA de los
-- dos—, no a la de 0001, que exigía el token en claro: con la columna vacía, esa
-- dejaría la tabla rechazando todos los enlaces nuevos.
ALTER TABLE shares DROP CONSTRAINT IF EXISTS shares_link_token;
ALTER TABLE shares ADD CONSTRAINT shares_link_token
    CHECK (kind <> 'link' OR token IS NOT NULL OR token_hash IS NOT NULL);
