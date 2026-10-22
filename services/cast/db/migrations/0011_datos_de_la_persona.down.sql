-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP POLICY IF EXISTS reports_de_quien_denuncia ON reports;
DROP POLICY IF EXISTS person_blocks_platform_merge ON person_blocks;
ALTER TABLE person_blocks DROP COLUMN IF EXISTS subject;
