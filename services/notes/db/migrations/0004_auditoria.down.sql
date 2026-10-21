-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira la traza de auditoría. Ninguna otra tabla la referencia, así que el
-- servicio sigue funcionando sin ella: lo que se pierde es saber quién publicó,
-- despublicó o destruyó algo.
--
-- sytue:destructive-ok la tabla solo se escribe desde este servicio y nada
-- depende de sus filas; volver atrás en una versión que aún no la usaba es
-- exactamente para lo que existe este fichero. Aun así, un `down` en producción
-- BORRA la traza de forma irreversible: si hay que conservarla para una
-- auditoría en curso, cópiala antes.

DROP TABLE IF EXISTS audit_log;
