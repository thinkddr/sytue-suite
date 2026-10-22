-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte 0017_api_token_project (el índice cae con la columna).

ALTER TABLE api_tokens DROP COLUMN IF EXISTS project_id;
