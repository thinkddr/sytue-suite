-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

CREATE INDEX CONCURRENTLY IF NOT EXISTS messages_mailbox_idx
    ON messages (tenant, mailbox_id, received_at DESC) WHERE deleted_at IS NULL;
