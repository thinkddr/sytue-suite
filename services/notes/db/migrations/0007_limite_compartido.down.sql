-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira el estado del limitador compartido. El servicio sigue arrancando: sin
-- la tabla, el limitador del enlace público falla en cada consulta y el
-- middleware está configurado FAIL-CLOSED, así que la puerta pública se cierra
-- en vez de abrirse de par en par. Es el lado correcto en el que fallar, pero
-- deja los cuadernos publicados inaccesibles: revertir esto exige revertir
-- también el código que lo usa.
--
-- sytue:destructive-ok solo guarda contadores efímeros de ventana; perderlos
-- reinicia los cubos, no borra contenido de nadie.

DROP TABLE IF EXISTS platform_rate_limits;
