-- SPDX-License-Identifier: Apache-2.0
-- «Soltar la fuente» (ADR 0086): la única vuelta de una fuente frozen/private a
-- legacy, por sytue_private_storage_release, con su auditoría en `releases`.
-- Cambia el disparador v1 de la fila de estado por el v2 (mismo nombre).
-- sytue:private-storage-release-v1
SELECT sytue_private_storage_release_install('public.contacts_private_storage_states'::regclass);
