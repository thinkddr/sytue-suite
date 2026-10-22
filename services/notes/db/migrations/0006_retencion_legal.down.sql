-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira la retención legal. El servicio sigue funcionando: lo que se pierde es
-- la capacidad de decir «esto no se borra», y el recolector de la 0005 vuelve a
-- purgar TODO lo que haya cumplido su plazo.
--
-- sytue:destructive-ok la tabla solo la escribe este servicio y ninguna otra la
-- referencia. Pero léase esto antes de ejecutarlo en producción: si hay algún
-- hold ACTIVO, revertir esta migración deja sin protección el contenido que
-- estaba preservando, y la siguiente pasada del recolector —una hora después
-- como mucho— lo destruye de forma irreversible. Si hay un litigio en curso,
-- primero apaga la purga (`trashRetention: 0`) y luego revierte.

DROP POLICY IF EXISTS legal_holds_platform_merge   ON legal_holds;
DROP POLICY IF EXISTS legal_holds_maintenance      ON legal_holds;
DROP POLICY IF EXISTS legal_holds_tenant_isolation ON legal_holds;
DROP TABLE IF EXISTS legal_holds;
