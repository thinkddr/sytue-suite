-- Favoritas.
--
-- UNA COLUMNA EN `assets` y no una tabla aparte: una foto tiene un solo dueño
-- —`owner_sub` está en la propia fila— así que «favorita» es un atributo suyo,
-- no una relación. Una tabla de unión solo haría falta si varias personas
-- pudieran marcar la misma foto, y aquí no comparten fotos: comparten ficheros
-- en Drive, cada uno con su fila.
ALTER TABLE assets ADD COLUMN favorite boolean NOT NULL DEFAULT false;

-- Índice PARCIAL, con la MISMA expresión de orden que `assets_timeline`.
--
-- La vista de favoritas es la línea de tiempo con un filtro, y pagina con el
-- mismo cursor `(coalesce(taken_at, uploaded_at), id)`. Si el orden no coincide
-- exactamente, Postgres deja de poder usar el índice y ordena en memoria la
-- colección entera en cada desplazamiento. Parcial porque las favoritas son
-- pocas: el índice ocupa lo que ocupan ellas, no la biblioteca.
CREATE INDEX assets_favoritas ON assets (
    tenant, owner_sub, coalesce(taken_at, uploaded_at) DESC, id DESC
) WHERE favorite AND deleted_at IS NULL;
