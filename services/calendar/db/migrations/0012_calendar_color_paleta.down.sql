-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Vuelta atrás: solo el valor que puso la migración (en mayúsculas, como lo
-- escribe el servicio). El ciruela elegido desde la interfaz va en minúsculas
-- («#6b3e9e») y no se toca.
UPDATE calendars
   SET color = '#3B82F6', updated_at = now()
 WHERE color = '#6B3E9E';
