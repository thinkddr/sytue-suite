-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Etiquetas de sensibilidad sobre FICHEROS (paridad Purview / Google Drive labels).
--
-- La TAXONOMÍA vive en Sytue ID (mig 0024 de id: sensitivity_labels por organización).
-- Aquí NO se replica la tabla ni se guarda una FK cross-servicio: el fichero lleva un
-- SNAPSHOT DENORMALIZADO de la etiqueta que se le puso. Motivos:
--
--   1. Drive decide SIN llamar a Sytue ID en el camino caliente. El bloqueo de enlaces
--      públicos se resuelve con la fila del propio fichero: sin salto de red, sin
--      dependencia de disponibilidad y sin latencia añadida al compartir.
--   2. La etiqueta sigue siendo LEGIBLE aunque la taxonomía cambie o se borre. Un
--      fichero marcado «Confidencial» hace un año se sigue leyendo como tal.
--   3. Un cambio posterior de la taxonomía NO reescribe el histórico. Re-etiquetar es
--      una acción explícita del admin, no un efecto colateral silencioso.
--
-- label_id es el uuid de la etiqueta EN Sytue ID, guardado como TEXTO: es una
-- referencia informativa a otro servicio (para «¿sigue existiendo?» y para re-resolver),
-- nunca una clave foránea. '' = fichero SIN etiquetar (el estado por defecto: esta ola
-- NO implementa etiquetado obligatorio ni clasificación automática).
--
-- label_rank y label_block_external_share son parte del snapshot A PROPÓSITO: la
-- POLÍTICA que se aplicó al etiquetar viaja con el fichero. Si mañana el admin quita el
-- bloqueo de la etiqueta en la taxonomía, los ficheros ya etiquetados lo conservan
-- hasta que se re-etiqueten (fail-closed: nunca se afloja una restricción sola).
--
-- Sin índice nuevo: la única consulta que filtra por etiqueta es la búsqueda de
-- eDiscovery, que ya acota por (tenant, owner_sub) —muy selectivo— y es un camino frío
-- de administración. ponytail: añadir un índice parcial (tenant, label_id) si aparece
-- un listado «todo lo etiquetado como X» a escala.

ALTER TABLE files
    ADD COLUMN label_id                   text     NOT NULL DEFAULT '',
    ADD COLUMN label_name                 text     NOT NULL DEFAULT '',
    ADD COLUMN label_rank                 smallint NOT NULL DEFAULT 0,
    ADD COLUMN label_block_external_share boolean  NOT NULL DEFAULT false;
