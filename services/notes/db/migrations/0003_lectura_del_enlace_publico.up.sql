-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- ENLACE PÚBLICO: política PERMISSIVE acotada a `app.public_link='on'`.
--
-- Un cuaderno publicado se lee SIN sesión, así que en esa petición no hay
-- organización que fijar: `app.tenant` viene vacío y la política de aislamiento
-- —`tenant = current_setting('app.tenant', true)`— evalúa a NULL y NO devuelve
-- ninguna fila. Es la degradación correcta de RLS deny-by-default y también el
-- motivo por el que el enlace público no resolvía nada: sin esta política, la
-- respuesta era «no existe» para TODOS los tokens, incluidos los buenos.
--
-- El alcance es deliberadamente estrecho, en tres verjas a la vez:
--   · Variable propia `app.public_link`, distinta de `app.merge` y de cualquier
--     otra: ninguna transacción existente gana alcance nuevo.
--   · Solo la fija `store.GetNotebookByPublicToken`, que es la única entrada sin
--     sesión del servicio.
--   · Y aun con la variable puesta, la política solo ve cuadernos PUBLICADOS y
--     con token; el token de 32 bytes sigue siendo la credencial.
--
-- Solo `notebooks`. Las NOTAS del cuaderno se leen después, ya sabiendo a qué
-- organización pertenece, con `app.tenant` fijado como cualquier otra consulta:
-- ahí la política de aislamiento normal basta y no hace falta relajar nada.

DROP POLICY IF EXISTS notebooks_enlace_publico ON notebooks;
CREATE POLICY notebooks_enlace_publico ON notebooks
    FOR SELECT
    USING (
        current_setting('app.public_link', true) = 'on'
        AND visibility = 'public'
        AND length(public_token) > 0
        AND deleted_at IS NULL
    );
