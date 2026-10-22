-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Bajar esto borra los scripts, sus versiones y el registro de ejecuciones.
DROP TABLE IF EXISTS doc_script_runs;
DROP TABLE IF EXISTS doc_script_versions;
DROP FUNCTION IF EXISTS doc_script_versions_inmutable();
DROP TABLE IF EXISTS doc_scripts;
