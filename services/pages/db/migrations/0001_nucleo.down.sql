-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- sytue:destructive-ok deshace la migración inicial de Sytue Pages: sin ella no hay servicio que use estas tablas.

DROP TABLE IF EXISTS events_outbox;
DROP TABLE IF EXISTS platform_rate_limits;
DROP TABLE IF EXISTS altcha_used;
DROP TABLE IF EXISTS assets;
DROP TABLE IF EXISTS redirects;
DROP TABLE IF EXISTS publications;
DROP TABLE IF EXISTS site_versions;
DROP TABLE IF EXISTS retired_slugs;
DROP TABLE IF EXISTS sites;
