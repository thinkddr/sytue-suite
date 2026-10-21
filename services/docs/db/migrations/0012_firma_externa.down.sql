-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Las peticiones a gente de fuera se pierden: sin la columna parecerían de dentro.
-- Sus enlaces de Drive se quedan hasta caducar (30 días como mucho), y su token, que
-- solo estaba aquí, se va con la fila: nadie puede usarlos.
DROP FUNCTION IF EXISTS docs_firmas_pendientes_para(text);
DROP FUNCTION IF EXISTS docs_firmas_de_fuera(text, text);
DROP FUNCTION IF EXISTS docs_firma_anfitrion(uuid, text);
SELECT set_config('app.merge', 'on', true);
DELETE FROM pdf_peticiones_firma WHERE externo;
DROP INDEX IF EXISTS pdf_peticiones_firma_externo_idx;
ALTER TABLE pdf_peticiones_firma
    DROP CONSTRAINT IF EXISTS pdf_peticiones_firma_externo_chk,
    DROP CONSTRAINT IF EXISTS pdf_peticiones_firma_externo_huella_chk,
    DROP CONSTRAINT IF EXISTS pdf_peticiones_firma_enlace_cerrada_chk,
    DROP COLUMN IF EXISTS ocupada, DROP COLUMN IF EXISTS firmado_huella, DROP COLUMN IF EXISTS huella,
    DROP COLUMN IF EXISTS copia_ruta, DROP COLUMN IF EXISTS copia,
    DROP COLUMN IF EXISTS enlace_share, DROP COLUMN IF EXISTS enlace,
    DROP COLUMN IF EXISTS externo;
