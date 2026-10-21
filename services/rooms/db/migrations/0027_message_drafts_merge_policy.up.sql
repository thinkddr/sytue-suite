-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Política de fusión para message_drafts (0026).
--
-- Toda tabla con columna `tenant` la necesita: sin ella, mover una organización
-- aborta en el preflight de la fusión —o peor, la da por completada dejando atrás
-- los borradores, que reaparecerían vacíos en la organización destino—. Va en su
-- propio fichero y no dentro de la 0026 porque esa ya puede estar aplicada en
-- algún entorno: reescribirla cambiaría su suma de comprobación.

DROP POLICY IF EXISTS message_drafts_platform_merge ON message_drafts;
CREATE POLICY message_drafts_platform_merge ON message_drafts
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
