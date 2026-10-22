-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Lo que Cast guarda de una PERSONA entra en «Exportar tus datos» y en «Borrar
-- mi cuenta» (ADR 0074).
--
-- BORRAR. Sus bloqueos personales se borran con la cuenta, como `user_blocks` de
-- Rooms: por el mismo camino (services/id/internal/merge, DatosDeLaPersona), que
-- busca `subject` y entra con `app.merge`. La columna se llama `blocker_sub` y
-- renombrarla rompería a los pods de la versión anterior durante el despliegue,
-- así que `subject` es una columna GENERADA con el mismo valor: no la escribe
-- nadie y no cambia nada de lo que ya hay. Los bloqueos que OTROS le pusieron a
-- ella son de esas personas, y se quedan en su lista.
ALTER TABLE person_blocks ADD COLUMN subject text GENERATED ALWAYS AS (blocker_sub) STORED;

CREATE POLICY person_blocks_platform_merge ON person_blocks
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

-- EXPORTAR. Las denuncias que ELLA puso, de cualquier organización: viven en la
-- del contenido denunciado. Se leen con `app.cast_persona` = su sub, como sus
-- bloqueos, y solo las suyas; nunca las que otros pusieron contra ella. Una
-- denuncia sin cuenta (`reporter_sub` vacío) no es de nadie.
CREATE POLICY reports_de_quien_denuncia ON reports
    FOR SELECT
    USING (reporter_sub <> '' AND reporter_sub = current_setting('app.cast_persona', true));
