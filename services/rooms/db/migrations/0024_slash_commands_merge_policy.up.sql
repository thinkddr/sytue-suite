-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Política de fusión para slash_commands (0023).
--
-- Misma historia que en mail: la 0021 barrió las tablas de entonces y esta nació
-- dos migraciones después, así que la fusión de organizaciones abortaba en el
-- preflight. Sin esta política, mover la organización habría dejado atrás sus
-- comandos —el bot deja de responder a «/desplegar» y nadie relaciona una cosa
-- con la otra—, o peor, el motor lo habría dado por completado.

DROP POLICY IF EXISTS slash_commands_platform_merge ON slash_commands;
CREATE POLICY slash_commands_platform_merge ON slash_commands
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
