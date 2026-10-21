-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
BEGIN;
DROP INDEX IF EXISTS geoip_networks_net_idx;
DROP TABLE IF EXISTS geoip_networks;
COMMIT;
