-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- LOS INVITADOS DE OTRA ORGANIZACIÓN A UN EVENTO ENTRAN EN SU REUNIÓN (decidido por el
-- dueño el 2026-10-16: «Que entren como invitados»).
--
-- La reunión de un evento vive en la organización que lo convocó (la anfitriona) y quien
-- viene de fuera llega con un token de la SUYA. Como en las conversaciones con gente de
-- fuera (ADR 0040), la petición se lleva a la anfitriona sin sus roles; para eso hay que
-- saber en qué organización vive la sala, y es la única lectura entre organizaciones:
-- por la política de mantenimiento, de solo lectura y SOLO para las salas de un evento.
-- Si entra lo decide después la verja de siempre (Calendar dice si está invitado, o trae
-- la clave del enlace).
CREATE POLICY rooms_eventos_maintenance ON rooms FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on'
           AND metadata->'source'->>'kind' = 'calendar');
