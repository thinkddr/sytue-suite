-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Preferencias de canal por usuario: favorito y silenciado. Viven en la fila de
-- pertenencia (channel_members) porque son estado per-(usuario, canal). El sidebar
-- las lee en ListChannels vía LEFT JOIN a la propia membresía. Silenciar excluye el
-- canal del agregado de notificación (el cliente aplica el flag); el no-leído se
-- sigue mostrando, atenuado.

ALTER TABLE channel_members ADD COLUMN favorite boolean NOT NULL DEFAULT false;
ALTER TABLE channel_members ADD COLUMN muted    boolean NOT NULL DEFAULT false;

-- Listar los favoritos de un usuario sin escanear toda su membresía (el sidebar
-- los agrupa arriba). Índice parcial: solo las filas marcadas.
CREATE INDEX channel_members_favorites ON channel_members (tenant, subject) WHERE favorite;
