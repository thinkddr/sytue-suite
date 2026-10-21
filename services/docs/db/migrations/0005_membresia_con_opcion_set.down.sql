-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sin vuelta atrás. Quitar la opción SET devolvería el servicio al estado en que
-- ninguna consulta se puede ejecutar: el store asume `docs_app` en cada
-- transacción. Revertir esto no restaura una configuración anterior legítima,
-- rompe el servicio.

SELECT 1;
