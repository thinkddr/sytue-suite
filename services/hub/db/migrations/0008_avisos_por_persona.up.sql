-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Los topes por PERSONA en todas las organizaciones (notify: TopeAvisosPersonaPorHora)
-- cuentan por `subject` sin `tenant`, y el índice del feed empieza por `tenant`: sin
-- este, cada aviso en vivo recorrería la tabla. CONCURRENTLY para no bloquear las
-- escrituras mientras se construye; por eso va solo en su fichero (no admite
-- transacción). Si se corta a medias, el índice queda INVÁLIDO y la migración «dirty»:
-- relanzar no lo arregla (IF NOT EXISTS lo da por hecho). Receta en
-- docs/operations.md §1.14b (indisvalid → DROP INDEX CONCURRENTLY → migrate force).
CREATE INDEX CONCURRENTLY IF NOT EXISTS notifications_por_persona ON notifications (subject, created_at);
