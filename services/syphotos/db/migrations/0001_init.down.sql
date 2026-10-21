-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Vuelta atras del esquema base de Sytue Photos.
--
-- No borra NADA del almacen de objetos ni de Drive: las derivadas son
-- regenerables y los originales no son de este servicio.

DROP TABLE IF EXISTS album_assets;
DROP TABLE IF EXISTS albums;
DROP TABLE IF EXISTS edits;
DROP TABLE IF EXISTS assets;
