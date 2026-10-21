-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
DELETE FROM data_connections WHERE kind IN ('gateway', 'bases');
ALTER TABLE data_connections DROP CONSTRAINT IF EXISTS data_connections_kind_chk;
ALTER TABLE data_connections ADD CONSTRAINT data_connections_kind_chk
    CHECK (kind IN ('rest', 'postgres'));
