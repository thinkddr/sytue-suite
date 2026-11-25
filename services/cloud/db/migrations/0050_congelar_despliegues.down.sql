-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP POLICY IF EXISTS deploy_freezes_platform_merge ON deploy_freezes;
DROP POLICY IF EXISTS deploy_freezes_tenant ON deploy_freezes;
DROP TABLE IF EXISTS deploy_freezes;
