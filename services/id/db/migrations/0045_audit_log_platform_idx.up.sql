-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Leer el registro de la PLATAFORMA sin recorrer el de todo el mundo.
--
-- Lo que hace un admin global se escribía desde el principio, pero no se podía
-- leer: la mitad de las entradas llevan el org_id de la organización tocada —y
-- solo se veían desde la consola de ESA organización, donde un admin global no
-- entra— y la otra mitad (conceder alcance global, elevarse, retirar plazas) no
-- lleva ninguno, así que no aparecía en ninguna parte.
--
-- Los índices que ya había son por org (0008) y por fecha (0006). Ninguno sirve
-- aquí: `action LIKE 'platform.%'` sobre el índice de fecha obliga a recorrer
-- toda la tabla hacia atrás hasta juntar cien filas de plataforma, y son una
-- minoría diminuta frente a los accesos de los usuarios. Índice PARCIAL: solo
-- indexa esas filas, así que ocupa lo que ocupan ellas y las sirve ya ordenadas.
--
-- El predicado tiene que ser IDÉNTICO al de la consulta (platformAuditWhere):
-- PostgreSQL solo usa un índice parcial cuando puede demostrar que el WHERE de
-- la consulta implica el suyo, y no hace análisis de patrones LIKE. Si uno de
-- los dos cambia sin el otro, no falla nada: vuelve el seq scan.

CREATE INDEX audit_log_platform_idx
    ON audit_log (occurred_at DESC, id DESC)
    WHERE action LIKE 'platform.%';
