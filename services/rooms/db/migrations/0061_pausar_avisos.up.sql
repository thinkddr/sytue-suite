-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- «PAUSAR AVISOS» CON DURACIÓN (Slack «Pause notifications»): hasta cuándo están en
-- pausa los avisos de una persona. NULL o un instante ya pasado = no hay pausa; la
-- expiración no necesita ningún trabajo que la limpie, porque el cliente compara con
-- su reloj, igual que la ventana «No molestar» de esta misma fila (mig. 0012).
--
-- ADD COLUMN sin DEFAULT es solo metadatos: no reescribe la tabla.
ALTER TABLE user_notify_prefs ADD COLUMN paused_until timestamptz;
