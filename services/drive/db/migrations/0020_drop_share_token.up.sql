-- Contract del par expand→contract que abrió 0018: se borra `shares.token`.
--
-- sytue:destructive-ok el expand fue 0018 y su versión ya no está viva: v2.0.0
-- está desplegada en producción (Helm rev. 59) y el servicio lee y escribe
-- únicamente `token_hash` desde entonces. Esta columna es el ÚLTIMO sitio donde
-- quedaba el secreto en claro.
--
-- Por qué esto importa y no es limpieza cosmética: un enlace público de Drive es
-- una credencial portadora —quien tiene la cadena, abre el fichero—. Mientras la
-- columna exista, los enlaces creados ANTES de 0018 siguen ahí en texto plano, y
-- cualquier vía que lea la tabla los entrega: una copia de seguridad, un volcado
-- para depurar, un SELECT * de alguien con acceso al motor. El hash no se puede
-- deshacer; el token, no hace falta deshacerlo.
--
-- CUIDADO con el orden, que aquí está el peligro de verdad: el CHECK
-- `shares_link_token` que dejó 0019 NOMBRA la columna
--
--     CHECK (kind <> 'link' OR token IS NOT NULL OR token_hash IS NOT NULL)
--
-- y PostgreSQL borra en CASCADA los constraints que referencian una columna que
-- se elimina. Es decir: un `DROP COLUMN` a secas se llevaría por delante la verja
-- que impide crear un enlace público SIN credencial —una URL que abre cualquiera
-- que dé con la ruta— y no lo diría por ningún sitio. La tabla se quedaría sin la
-- comprobación y el esquema seguiría pareciendo correcto.
--
-- Por eso se vuelve a poner acto seguido, ya sin la mención al token en claro.
-- Es exactamente lo que 0019 dejó escrito que había que hacer: «esta restricción
-- se queda tal cual y basta con quitar el OR token IS NOT NULL».
ALTER TABLE shares DROP COLUMN IF EXISTS token;

ALTER TABLE shares DROP CONSTRAINT IF EXISTS shares_link_token;
ALTER TABLE shares ADD CONSTRAINT shares_link_token
    CHECK (kind <> 'link' OR token_hash IS NOT NULL);
