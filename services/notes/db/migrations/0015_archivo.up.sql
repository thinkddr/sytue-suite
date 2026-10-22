-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
-- Archivo de cuadernos: fuera del árbol y de la búsqueda, pero intactos. La marca
-- es una fecha y no un booleano: archivar una rama le pone a todos la MISMA, y
-- desarchivar solo devuelve los que la comparten (un hijo archivado antes sigue
-- archivado). Los diarios no se archivan: ya viven fuera del árbol.
ALTER TABLE notebooks ADD COLUMN archived_at timestamptz;
ALTER TABLE notebooks ADD CONSTRAINT notebook_diario_sin_archivo CHECK (
    archived_at IS NULL OR daily_date = ''
);
