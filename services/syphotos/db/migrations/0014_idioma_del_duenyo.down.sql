-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
-- sytue:destructive-ok revierte la columna owner_locale creada en 0014

ALTER TABLE assets DROP COLUMN IF EXISTS owner_locale;
