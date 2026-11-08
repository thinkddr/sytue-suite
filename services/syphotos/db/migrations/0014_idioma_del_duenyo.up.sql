-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- IDIOMA DEL DUEÑO, para el lote de IA.
--
-- El lote que describe las fotos (`vectores-serverless`) no es una petición de
-- nadie: va con un token de servicio, que no trae el idioma del perfil. Sin él,
-- la descripción y las etiquetas que escribe salen en el idioma que se le antoje
-- al modelo —en la práctica, inglés— dentro de una cuenta en español.
--
-- Se guarda el `locale` del token de quien da de alta la foto. El lote usa el de
-- la foto MÁS RECIENTE de esa persona, no el de cada foto: así, quien cambia de
-- idioma lo ve también en las fotos viejas que se vuelvan a describir.
-- Vacío = no se sabía (fotos anteriores a esta migración, o un token sin el
-- claim); entonces el lote no fija idioma.

ALTER TABLE assets ADD COLUMN owner_locale text NOT NULL DEFAULT '';
