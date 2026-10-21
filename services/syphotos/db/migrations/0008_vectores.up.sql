-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- EL VECTOR DE CADA FOTO: qué se ve en ella, en números.
--
-- Lo escribe el lote de GPU (deploy/fotos-gpu) pasando la miniatura por SigLIP
-- 2. Con dos vectores se puede medir si dos fotos se parecen, que es lo que
-- hace falta para «más como esta», para agrupar y para no repetir.
--
-- `real[]` Y NO pgvector, a propósito y con fecha de caducidad. La suite no usa
-- pgvector en ningún sitio, así que meterlo obliga a cambiar la imagen de
-- PostgreSQL de TODOS los servicios por una funcionalidad de uno. A esta escala
-- no hace falta: comparar unos miles de vectores es un bucle en Go de
-- milisegundos. Cuando una biblioteca llegue a cientos de miles, el índice
-- aproximado de pgvector empieza a ganar, y ese es el momento de migrar: la
-- columna se convierte con un `ALTER ... USING`, sin volver a vectorizar nada.
--
-- SE GUARDA TAMBIÉN QUÉ MODELO lo escribió. Dos vectores de modelos distintos
-- no se pueden comparar —viven en espacios distintos— así que compararlos daría
-- parecidos inventados. Con el nombre delante, cambiar de modelo es volver a
-- vectorizar lo que tenga el nombre viejo, y mientras tanto cada cual se compara
-- con los suyos.
ALTER TABLE assets
    ADD COLUMN embedding       real[],
    ADD COLUMN embedding_model text NOT NULL DEFAULT '';

-- El lote pregunta «qué falta por vectorizar» y esa consulta recorre TODAS las
-- organizaciones, así que sin índice sería un recorrido completo cada semana.
-- Parcial: lo que ya tiene vector no vuelve a mirarse.
CREATE INDEX assets_sin_vector ON assets (id)
    WHERE embedding IS NULL AND deleted_at IS NULL;

-- Las dos o ninguna: un vector sin modelo no se puede comparar con nada, y un
-- modelo sin vector es una etiqueta que miente.
ALTER TABLE assets ADD CONSTRAINT assets_vector_con_modelo
    CHECK ((embedding IS NULL) = (embedding_model = ''));
