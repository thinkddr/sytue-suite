-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE access_policies DROP COLUMN require_trusted_device;

DROP TABLE devices;
