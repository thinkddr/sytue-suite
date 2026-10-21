-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Los calendarios creados sin color recibían «#3B82F6», un azul que no está en
-- la paleta de la interfaz (los diez colores de producto) y que en tema oscuro
-- chilla junto al latón. El servicio ya asigna el ciruela del producto
-- («#6B3E9E») a lo nuevo; esto iguala lo que ya existía. Es seguro porque ese
-- azul no se puede elegir desde la interfaz: solo puede venir del viejo valor
-- por defecto, nunca de una decisión de la persona.
UPDATE calendars
   SET color = '#6B3E9E', updated_at = now()
 WHERE color = '#3B82F6';
