-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Las conexiones de los consumidores que dejan de existir se revocan y se borran: sin su
-- valor en el CHECK no caben, y un token que nadie puede ya gestionar no debe seguir leyendo.
DELETE FROM base_connections WHERE consumer <> 'calcus';
ALTER TABLE base_connections DROP CONSTRAINT base_connections_consumer_check;
ALTER TABLE base_connections ADD CONSTRAINT base_connections_consumer_check CHECK (consumer IN ('calcus'));
