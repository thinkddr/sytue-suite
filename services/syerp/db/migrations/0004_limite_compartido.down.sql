-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira el estado del limitador compartido. El servicio sigue arrancando, pero
-- el webhook de Stripe vuelve a contestar 503 a todo: el middleware es
-- fail-closed, que es el lado correcto en el que fallar —mejor no cobrar que
-- aceptar cobros sin límite— pero deja el conector inservible. Revertir esto
-- exige revertir también el código que lo usa.
--
-- sytue:destructive-ok solo guarda contadores efímeros de ventana; perderlos
-- reinicia los cubos, no borra datos de nadie.

DROP TABLE IF EXISTS platform_rate_limits;
