-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Reparar el alta guiada de quien venía de una organización ya absorbida.
--
-- Hasta el arreglo de `MergeIdentity`, fusionar una organización movía las
-- pertenencias y borraba las del origen, pero dejaba `onboarding_progress.org_id`
-- nombrando a la organización absorbida. El asistente de alta re-verifica ser
-- OWNER de la org de su propia fila contra la BD en cada paso, así que a partir
-- de la fusión respondía 403 «No eres propietario de esta organización»; y como
-- el portal manda a /bienvenida a quien no tenga el alta completada, el usuario
-- aterrizaba justo en la pantalla que ya no podía cargar.
--
-- El código corregido evita que vuelva a pasar, pero no repara lo ya fusionado:
-- eso es dato, y el dato se arregla aquí. Pasó en producción con corporate.
--
-- Se cierra además de reapuntarse, por lo mismo que hace la fusión: el alta
-- terminó de hecho —la persona ya está dentro de una organización operativa— y
-- dejarla abierta contra el destino tampoco funcionaría, porque a un `owner` del
-- origen la fusión lo entra al destino como `admin`, y el asistente exige OWNER.
--
-- IDEMPOTENTE y acotada a lo roto: solo toca filas que aún apuntan a una
-- organización marcada como fusionada. Repetirla no mueve nada, y no hay que
-- perseguir cadenas de fusiones porque `merged_into_org_id` guarda el destino
-- final de cada una.
--
-- COALESCE conserva la fecha de quien ya la tuviera: esto no reescribe cuándo se
-- dio de alta nadie.

UPDATE onboarding_progress op
SET org_id       = o.merged_into_org_id,
    completed_at = COALESCE(op.completed_at, now()),
    updated_at   = now()
FROM organizations o
WHERE op.org_id = o.id
  AND o.status = 'merged'
  AND o.merged_into_org_id IS NOT NULL;
