-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Origen del documento cuando nació de un fichero que vive en otro sitio (hoy, un
-- fichero de Sytue Drive: 'drive:<uuid>').
--
-- POR QUÉ HACE FALTA: «Abrir con Sytue Docs» importa el fichero a un documento.
-- Sin recordar de dónde vino, cada clic en el mismo .docx crearía OTRA copia: a la
-- décima vez el usuario tiene diez documentos con el mismo nombre y las ediciones
-- repartidas entre ellos. Con el origen anotado, abrir dos veces lleva al mismo
-- documento, y es también lo que permite devolver los cambios al fichero.
ALTER TABLE documents ADD COLUMN IF NOT EXISTS source_ref text;

-- Un origen, un documento POR PERSONA. Deliberadamente no es único por tenant:
-- que dos personas abran el mismo fichero no puede meterlas en un documento cuyo
-- dueño es otra —heredarían sus permisos— así que cada una edita el suyo y Drive
-- guarda versiones. Compartir el mismo documento exige mapear los permisos de
-- Drive a los de Docs, que es otra ola.
CREATE UNIQUE INDEX IF NOT EXISTS documents_source_ref_uniq
    ON documents (tenant, owner_sub, source_ref)
    WHERE source_ref IS NOT NULL AND deleted_at IS NULL;
