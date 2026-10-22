-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Menciones a todo el canal (@channel / @all / @here) SIN fan-out de N inserciones.
--
-- Decisión de escala (cientos de miles de miembros): en vez de crear una fila de
-- mención por cada miembro del canal cuando alguien escribe @channel, se marca el
-- mensaje con `mentions_all`. El contador de menciones de cada usuario
-- (store.UnreadByChannel) cuenta esos mensajes por consulta indexada, exactamente
-- igual que las menciones @usuario vía el índice GIN — coste O(no leídos), no O(miembros).

ALTER TABLE messages ADD COLUMN mentions_all boolean NOT NULL DEFAULT false;

-- El subquery de menciones de UnreadByChannel cuenta, por canal y usuario, los
-- mensajes no leídos con mentions_all. Índice parcial (solo los marcados y vivos,
-- igual que el WHERE real) para que el OR mentions_all no degrade a seq scan.
CREATE INDEX messages_mentions_all
    ON messages (tenant, channel_id, created_at)
    WHERE mentions_all AND deleted_at IS NULL;
