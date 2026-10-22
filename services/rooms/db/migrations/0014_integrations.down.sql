-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP TABLE IF EXISTS incoming_webhooks;
DROP TABLE IF EXISTS outgoing_webhooks;
DROP TABLE IF EXISTS bots;
-- platform_rate_limits se crea con IF NOT EXISTS y lo comparten otros usos
-- potenciales; no se borra en la bajada para no romper esos consumidores.
