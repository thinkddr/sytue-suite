-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Son declaraciones que cada app repite entera en su siguiente subida o borrado:
-- bajar la tabla solo deja la cuota sin lo externo hasta la próxima declaración.
DROP TABLE IF EXISTS quota_external_usage;
