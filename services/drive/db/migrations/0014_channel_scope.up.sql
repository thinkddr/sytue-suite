-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- ÁMBITO DE CANAL: los adjuntos de Sytue Rooms dejan de vivir en «Mi unidad».
--
-- Problema (informe de paridad de Rooms, dominio «Ficheros y Sytue Drive»): un
-- fichero adjuntado a un mensaje —incluso de un canal PRIVADO o de un DM— se subía
-- a la carpeta RAÍZ del tenant, y «Mi unidad» es por diseño un espacio COMPARTIDO
-- del tenant (ver el ponytail de store.ListRecent y la política de la mig 0001):
-- cualquier miembro de la organización podía descargarlo con GET /files/{id}/content
-- y lo veía en búsqueda, recientes y el explorador. Confidencialidad rota.
--
-- Solución: un TERCER ámbito de contenido, hermano del de unidades de equipo (mig
-- 0010). `channel_id` marca la carpeta/fichero como propiedad de un CANAL de Rooms
-- (opaco para Drive: es el uuid del canal en services/chat) y la GUC LOCAL
-- `app.channel` selecciona ese ámbito en la transacción:
--   vacío/ausente → «Mi unidad» (channel_id IS NULL),
--   un uuid       → SOLO los ficheros de ESE canal.
-- Efecto: un adjunto de canal es INVISIBLE en el ámbito personal sin tocar ni una
-- consulta — desaparece de raíz/hijos/búsqueda/recientes/favoritos/papelera/ZIP/
-- WebDAV/compat Google-Graph/shares/enlace público. La única puerta es la ruta
-- /api/v1/channels/{channelID}/files/… del servicio, que exige MEMBRESÍA DEL CANAL
-- verificada contra Sytue Chat en cada petición (deny-by-default, sin ACL replicada).
--
-- La política PERMISSIVE de mantenimiento (mig 0009, `app.maintenance='on'`) sigue
-- OR-eándose: el worker de auto-purga y el export de eDiscovery ven estos ficheros
-- (deliberado: retención y Vault/Purview son accesos privilegiados y auditados).
--
-- RETROCOMPATIBLE: `channel_id` nace NULL, así que TODO el contenido existente
-- —incluidos los adjuntos ya subidos— sigue exactamente donde estaba, con los
-- permisos que tenía. Los adjuntos ANTIGUOS siguen, por tanto, legibles por la
-- organización: esta migración corta la fuga para los NUEVOS. Repararlos exige un
-- backfill que cruza dos bases de datos (el canal de cada fichero solo lo sabe
-- services/chat), así que se hace como tarea de operación, no en la migración:
--
--   1) En la BD de CHAT, sacar los pares (file_id, channel_id) de los adjuntos:
--      psql "$CHAT_DSN" -Atc "COPY (
--          SELECT DISTINCT (a->>'file_id')::uuid, m.channel_id, m.tenant
--          FROM messages m, jsonb_array_elements(m.attachments) a
--          WHERE m.deleted_at IS NULL AND a->>'file_id' <> ''
--        ) TO STDOUT WITH (FORMAT csv)" > adjuntos.csv
--   2) En la BD de DRIVE, aplicarlos (la RLS no estorba: es el dueño de la tabla en
--      una sesión de mantenimiento; SET app.maintenance='on' si se usa el rol de app):
--      psql "$DRIVE_DSN" -c "CREATE TEMP TABLE bf(file_id uuid, channel_id uuid, tenant text);
--        \\copy bf FROM 'adjuntos.csv' WITH (FORMAT csv)
--        UPDATE files f SET channel_id = bf.channel_id
--          FROM bf WHERE f.id = bf.file_id AND f.tenant = bf.tenant
--                    AND f.channel_id IS NULL AND f.team_drive_id IS NULL;"
--   Nota: el backfill NO mueve la fila de carpeta (folder_id sigue apuntando a la
--   raíz del tenant, invisible ya en el ámbito personal); ese fichero deja de
--   ocupar nombre útil en la raíz pero conserva la fila del índice único de
--   nombres, así que un fichero personal nuevo con el MISMO nombre en la raíz daría
--   conflicto. Es la razón por la que los adjuntos NUEVOS van a una carpeta propia
--   por canal. Ejecutar en ventana de mantenimiento y verificar con:
--     SELECT count(*) FROM files WHERE channel_id IS NOT NULL;

ALTER TABLE folders ADD COLUMN channel_id uuid;
ALTER TABLE files   ADD COLUMN channel_id uuid;

-- Una sola carpeta por canal (raíz desligada del árbol personal, como la raíz de
-- una unidad de equipo): sostiene el ON CONFLICT del get-or-create idempotente.
CREATE UNIQUE INDEX folders_channel_root_idx ON folders (tenant, channel_id)
    WHERE channel_id IS NOT NULL AND deleted_at IS NULL;
-- Contenido vivo de un canal, indexado (listar/gatear sin barrer la tabla).
CREATE INDEX files_channel_idx ON files (tenant, channel_id)
    WHERE channel_id IS NOT NULL AND deleted_at IS NULL;

-- Aislamiento por tenant + ÁMBITO (unidad de equipo Y canal). `IS NOT DISTINCT
-- FROM` casa NULL con NULL; `nullif(...,'')` colapsa el valor ausente/'' a NULL
-- (SET LOCAL revierte al terminar la tx: sin fugas entre transacciones de la misma
-- conexión del pool). Un ámbito es EXCLUYENTE: el contenido de un canal no se ve
-- desde «Mi unidad» ni desde una unidad de equipo, ni al revés.
DROP POLICY folders_scope_isolation ON folders;
CREATE POLICY folders_scope_isolation ON folders
    USING (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid
       AND channel_id    IS NOT DISTINCT FROM nullif(current_setting('app.channel', true), '')::uuid)
    WITH CHECK (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid
       AND channel_id    IS NOT DISTINCT FROM nullif(current_setting('app.channel', true), '')::uuid);

DROP POLICY files_scope_isolation ON files;
CREATE POLICY files_scope_isolation ON files
    USING (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid
       AND channel_id    IS NOT DISTINCT FROM nullif(current_setting('app.channel', true), '')::uuid)
    WITH CHECK (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid
       AND channel_id    IS NOT DISTINCT FROM nullif(current_setting('app.channel', true), '')::uuid);
