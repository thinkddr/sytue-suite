-- Dónde se hizo la foto.
--
-- Grados decimales y no la terna grados/minutos/segundos del EXIF: lo que se
-- guarda es la posición, no cómo la escribió la cámara. `double precision` da
-- de sobra —siete decimales son un centímetro— y es lo que entienden PostGIS y
-- cualquier mapa si algún día hace falta.
--
-- LAS DOS O NINGUNA, que es lo que garantiza el analizador: media coordenada no
-- sitúa nada y dejaría la foto en el meridiano de Greenwich sin que nadie lo
-- pidiera.
ALTER TABLE assets
    ADD COLUMN latitude  double precision,
    ADD COLUMN longitude double precision,
    ADD CONSTRAINT assets_lugar_completo
        CHECK ((latitude IS NULL) = (longitude IS NULL)),
    ADD CONSTRAINT assets_lugar_en_rango
        CHECK (latitude IS NULL OR (latitude BETWEEN -90 AND 90 AND longitude BETWEEN -180 AND 180));
