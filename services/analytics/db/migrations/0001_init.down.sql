-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte 0001. DROP TABLE de analytics_events retira también sus particiones.

DROP FUNCTION IF EXISTS analytics_expire_partitions(int);
DROP FUNCTION IF EXISTS analytics_ensure_partitions(int);
DROP TABLE IF EXISTS analytics_daily_breakdown;
DROP TABLE IF EXISTS analytics_daily;
DROP TABLE IF EXISTS analytics_hourly;
DROP TABLE IF EXISTS analytics_events;
DROP TABLE IF EXISTS analytics_sites;
DROP TABLE IF EXISTS analytics_salts;
