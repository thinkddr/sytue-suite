-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
-- Removing this fence would make previously protected data writable by an old
-- backend. Correct forward; restoring an older database is an explicit recovery
-- operation outside ordinary application/schema rollback.
DO $$ BEGIN
    RAISE EXCEPTION 'Notes write-contract activation cannot be rolled back safely';
END $$;
