-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El barrido periódico de Sytue Cast (service/barrido.go): purga del almacén los
-- vídeos borrados hace más del plazo de gracia y elimina su fila, y da por
-- fallidas las subidas que se quedaron en `uploading`. Cruza organizaciones, así
-- que corre en una transacción de mantenimiento, como la cola del transcodificador.
--
-- Lo que cuelga del vídeo (personas, trabajos, espectadores, comentarios, chat)
-- tiene ON DELETE CASCADE desde 0001–0003: las acciones referenciales no pasan
-- por la RLS, así que basta con poder borrar la fila de `videos`.

CREATE POLICY videos_mantenimiento_borrado ON videos
    FOR DELETE
    USING (current_setting('app.maintenance', true) = 'on');

-- Los borrados pendientes de retirar, del más antiguo al más reciente.
CREATE INDEX IF NOT EXISTS videos_borrados ON videos (deleted_at, id) WHERE deleted_at IS NOT NULL;

-- Las subidas en curso, por antigüedad de su último cambio.
CREATE INDEX IF NOT EXISTS videos_subiendo ON videos (updated_at)
    WHERE status = 'uploading' AND deleted_at IS NULL;
