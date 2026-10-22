-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte 0011_compute_metering (orden inverso: índice antes que la columna).

DROP INDEX IF EXISTS deployments_compute_meter_idx;
ALTER TABLE deployments DROP COLUMN IF EXISTS metered_at;
