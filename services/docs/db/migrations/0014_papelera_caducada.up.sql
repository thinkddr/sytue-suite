-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- LA PAPELERA DE DOCS CADUCA A LOS 30 DÍAS, como la de Drive (decidido por el dueño,
-- 2026-09-28). El barrido mira TODAS las organizaciones a la vez para encontrar lo
-- caducado, y solo eso: esta política es de LECTURA. Cada documento se borra después
-- en una transacción de SU organización, con su RLS de siempre (ver la purga de
-- firmas de la 0007, que es el mismo patrón).
CREATE POLICY documents_papelera_caducada ON documents
    FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on');

-- Lo que está en la papelera, por antigüedad: el barrido no recorre la tabla entera.
CREATE INDEX documents_papelera_idx ON documents (deleted_at) WHERE deleted_at IS NOT NULL;
