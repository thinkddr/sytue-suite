-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revertir devuelve suspender y fusionar a una sola persona. Se pierde el
-- registro de quién pidió y quién aprobó cada una.
DROP TABLE IF EXISTS platform_approvals;
