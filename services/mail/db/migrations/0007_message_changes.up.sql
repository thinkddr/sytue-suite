-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Seguimiento de cambios para el refresco incremental de la bandeja abierta
-- (estilo JMAP Email/changes, RFC 8621 §4.3). Una marca `updated_at` que TODA
-- mutación del sobre toca (insertar, flags/keywords, mover, borrar, etiquetar) y
-- un índice por (account_id, updated_at, id) que permite:
--   * leer el estado actual de la cuenta en O(1) (última fila del rango), y
--   * paginar los cambios desde un cursor (updated_at, id) en O(nº de cambios),
-- sin volcar la bandeja entera en cada sondeo (escala a buzones de cientos de
-- miles de mensajes). El índice NO es parcial (no excluye deleted_at IS NOT NULL)
-- porque los cambios deben reportar también los borrados (destroyed).

ALTER TABLE messages ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

CREATE INDEX messages_changes_idx ON messages (account_id, updated_at, id);
