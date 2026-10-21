-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Barrido periódico del aprovisionamiento SCIM saliente.
--
-- La maquinaria de reconciliación existe y funciona (internal/scimout: da de alta,
-- actualiza y DESACTIVA en el SaaS a quien ya no está en Sytue), pero solo se encolaba
-- al activar la conexión y cuando un administrador pulsaba «Sincronizar». Es decir: dar
-- de baja a alguien en Sytue ID no llegaba al SaaS del cliente hasta que a alguien se le
-- ocurriera sincronizar a mano. El desaprovisionamiento existía y no se disparaba nunca,
-- que para un control de seguridad es lo mismo que no existir.
--
-- Para enumerar las conexiones activas de TODAS las organizaciones hace falta cruzar el
-- aislamiento por tenant. El comentario del código asumía que eso exigía un rol con
-- BYPASSRLS; no lo exige: el resto de la suite (cloud, mail, hub) lo resuelve con una
-- política PERMISSIVE que solo se activa cuando la sesión fija `app.maintenance = 'on'`
-- (pg.InMaintenanceTx). Las transacciones de negocio NUNCA fijan ese flag, así que el
-- aislamiento por organización queda intacto — y el rol de aplicación sigue sin poder
-- saltarse la RLS, que es lo que hace que un test verde signifique algo.
--
-- USING sin WITH CHECK: el barrido solo LEE (qué conexiones tocan) y encola en el
-- outbox, que no lleva RLS. No necesita escribir en app_instances.
CREATE POLICY app_instances_maintenance ON app_instances
    USING (current_setting('app.maintenance', true) = 'on');

-- El barrido lee solo las conexiones ACTIVAS. Sin este índice sería un seq scan de la
-- tabla entera en cada tick; con él, un index scan sobre una fracción de las filas.
CREATE INDEX app_instances_active_idx ON app_instances (status) WHERE status = 'active';
