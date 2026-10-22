-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Vuelve al alcance original. Los trabajos de los dominios nuevos y los de
-- salida se borran ANTES de reponer los CHECK: dejarlos haría que la restricción
-- no se pudiera crear y la reversión fallase a mitad, que es la peor forma de
-- revertir.

DELETE FROM migration_jobs
 WHERE direction <> 'in'
    OR source NOT IN ('m365', 'google')
    OR domain NOT IN ('drive', 'contacts');

ALTER TABLE migration_jobs DROP COLUMN IF EXISTS direction;

ALTER TABLE migration_jobs DROP CONSTRAINT IF EXISTS migration_jobs_source_check;
ALTER TABLE migration_jobs ADD CONSTRAINT migration_jobs_source_check
    CHECK (source IN ('m365', 'google'));

ALTER TABLE migration_jobs DROP CONSTRAINT IF EXISTS migration_jobs_domain_check;
ALTER TABLE migration_jobs ADD CONSTRAINT migration_jobs_domain_check
    CHECK (domain IN ('drive', 'contacts'));
