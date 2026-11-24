-- SPDX-License-Identifier: Apache-2.0
-- «Soltar la fuente» (ADR 0086) en Drive: la única vuelta de una fuente frozen/private
-- a legacy, por sytue_private_storage_release, con su auditoría en `releases`. Es lo
-- que deja empezar de nuevo a quien vació la bóveda de su Drive: la fuente vuelve a
-- los datos del servidor tal como se congelaron y la bóveda nueva los importa otra vez.
-- Cambia el disparador v1 de la fila de estado por el v2 (mismo nombre).
-- sytue:private-storage-release-v1
SELECT sytue_private_storage_release_install('public.drive_private_storage_states'::regclass);
