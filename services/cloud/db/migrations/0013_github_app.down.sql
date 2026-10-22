-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP TABLE IF EXISTS github_pr_comments;
ALTER TABLE projects DROP COLUMN IF EXISTS github_installation_id;
