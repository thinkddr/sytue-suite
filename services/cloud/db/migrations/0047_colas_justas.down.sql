-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)

DROP INDEX IF EXISTS domains_unique_hostname_proyecto;
DROP INDEX IF EXISTS domains_unique_hostname_verificado;
DELETE FROM domains d USING domains o
 WHERE lower(d.hostname) = lower(o.hostname) AND d.status = 'pending' AND d.id <> o.id;
CREATE UNIQUE INDEX domains_unique_hostname ON domains (lower(hostname));
ALTER TABLE managed_services DROP COLUMN IF EXISTS checked_at;
ALTER TABLE domains DROP COLUMN IF EXISTS checked_at;
