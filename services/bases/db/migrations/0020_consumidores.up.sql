-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Conector de solo lectura: además de Calcus, consumen Sytue Drive (exportar una vista a
-- una carpeta), SyERP (fuente de un informe) y Sytue Web (sección de un sitio). Una
-- conexión es de UN consumidor: el token de una no sirve en otro (lo comprueba la lectura).
--
-- La numeración salta a 0020: el rango 0020–0029 es el de las integraciones de bases-2,
-- para no chocar con las piezas de núcleo y de colaboración, que numeran a la vez.
ALTER TABLE base_connections DROP CONSTRAINT base_connections_consumer_check;
ALTER TABLE base_connections ADD CONSTRAINT base_connections_consumer_check
    CHECK (consumer IN ('calcus', 'drive', 'erp', 'web'));
