-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
ALTER TABLE notebooks DROP CONSTRAINT notebook_diario_sin_archivo;
ALTER TABLE notebooks DROP COLUMN archived_at;
