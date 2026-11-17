-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- El índice de 0001 ordenaba por received_at y ya no lo usa ninguna consulta de lista
-- (ordenan por la fecha de orden de la 0037, con el índice de la 0038). Se retira para
-- no pagar su mantenimiento en cada escritura. CONCURRENTLY y solo en su fichero, como
-- la 0038: un DROP INDEX normal toma un bloqueo exclusivo sobre `messages`.
DROP INDEX CONCURRENTLY IF EXISTS messages_mailbox_idx;
