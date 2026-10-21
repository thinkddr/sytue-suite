-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP TRIGGER IF EXISTS records_historial ON records;
DROP FUNCTION IF EXISTS historial_de_records();
DROP TABLE IF EXISTS historial;
