-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira el estado del limitador compartido. Sin la tabla, el limitador falla en
-- cada consulta y el middleware, FAIL-CLOSED, cierra lo anónimo y `/media` con
-- 503: revertir esto exige revertir también el código que lo usa.
--
-- sytue:destructive-ok solo guarda contadores efímeros de ventana; perderlos
-- reinicia los cubos, no borra contenido de nadie.

DROP INDEX IF EXISTS platform_rate_limits_updated_at_idx;
DROP TABLE IF EXISTS platform_rate_limits;
