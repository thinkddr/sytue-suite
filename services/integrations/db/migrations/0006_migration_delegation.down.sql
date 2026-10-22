-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte 0006: vuelve al modelo «el contenido aterriza en la cuenta de quien
-- lanza el trabajo». La columna del token vuelve VACÍA (los tokens ya no existen:
-- son de vida corta y no se guardaron), así que los trabajos en curso habrá que
-- reanudarlos para que vuelvan a sellar uno.

ALTER TABLE migration_jobs ADD COLUMN dest_token_enc text NOT NULL DEFAULT '';
ALTER TABLE migration_jobs RENAME COLUMN target_user TO subject;

DROP TABLE IF EXISTS migration_delegates;
