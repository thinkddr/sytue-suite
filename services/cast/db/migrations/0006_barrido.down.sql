-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Vuelta atrás del barrido: sin la política, el binario que barre falla al
-- eliminar filas (lo registra y reintenta en la pasada siguiente); no pierde nada.

DROP INDEX IF EXISTS videos_subiendo;
DROP INDEX IF EXISTS videos_borrados;
DROP POLICY IF EXISTS videos_mantenimiento_borrado ON videos;
