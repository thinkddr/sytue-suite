-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- El tope de reconstruidos por persona (TopeHistoricosPorPersona) cuenta los suyos en
-- todas las organizaciones. Parcial: solo las filas reconstruidas. CONCURRENTLY, como la 0008,
-- y con su misma receta si se corta a medias.
CREATE INDEX CONCURRENTLY IF NOT EXISTS notifications_historicos_por_persona
    ON notifications (subject) WHERE clave_origen IS NOT NULL;
