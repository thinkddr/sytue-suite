-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sin vuelta atrás, a propósito.
--
-- La subida repara filas que apuntaban a una organización absorbida, y no guarda
-- a cuál apuntaba cada una: deshacerlo exigiría adivinar de qué organización
-- venía cada persona entre todas las que se fusionaron en el mismo destino.
-- Devolverlas «a alguna» sería reintroducir el 403 en cuentas al azar.
--
-- El estado al que lleva la subida es además el correcto con o sin este cambio
-- desplegado: una fila de alta que apunta a la organización donde la persona
-- está de verdad. No hay nada que revertir.

SELECT 1;
