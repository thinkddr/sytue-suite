-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- TOKEN EXCHANGE (RFC 8693) con impersonación ACOTADA Y AUDITADA.
--
-- Un client de confianza (p. ej. el runner de migraciones desde M365/Google) canjea
-- su propio access token por otro que actúa EN NOMBRE de un usuario de SU
-- organización: `sub` del usuario, `act.sub` del client, TTL de minutos y scopes
-- recortados. Sin esto, un trabajo de migración escribe con el Bearer del admin y
-- el contenido aterriza en la cuenta del admin, no en la de cada usuario.
--
-- Esto es IMPERSONACIÓN: la puerta es DENY-BY-DEFAULT y vive aquí, en la fila del
-- client, no en un scope ni en una config de despliegue.
--
--   may_impersonate         el client puede canjear. false por defecto.
--   may_impersonate_admins  además, puede impersonar a owner/admin de la org.
--                           false por defecto: el caso normal (migrar buzones y
--                           ficheros) NO necesita actuar como un administrador.
--
-- QUIÉN PUEDE ACTIVARLAS: NADIE por API. Ni el registro dinámico (RFC 7591) ni el
-- CRUD admin de clients de la Console escriben estas columnas — sus INSERT/UPDATE
-- ni las mencionan, así que quedan en su DEFAULT false. Se activan FUERA DE BANDA
-- (SQL del operador de plataforma, con su propio control de cambios).
--
-- Es deliberado: si un admin de org pudiera marcarse un client como delegable desde
-- la Console, cualquier compromiso de una sesión de admin se convertiría en
-- impersonación silenciosa y permanente de TODA la organización — la escalada
-- exacta que este diseño existe para impedir. El coste (un paso manual al habilitar
-- una migración) es despreciable frente a eso, y deja rastro en el control de
-- cambios de la plataforma además del audit_log de cada canje.
--
-- INVARIANTES (CHECK, no confianza en la aplicación):
--   1) un client delegable DEBE estar acotado a una organización (org_id NOT NULL).
--      Los clients globales de la suite (org_id NULL, mig 0002/0017) no tienen
--      "misma organización" contra la que acotar: podrían impersonar en cualquier
--      tenant. Se prohíbe por construcción.
--   2) may_impersonate_admins implica may_impersonate (no hay delegación de
--      administradores sin delegación).

ALTER TABLE oauth_clients
    ADD COLUMN may_impersonate        boolean NOT NULL DEFAULT false,
    ADD COLUMN may_impersonate_admins boolean NOT NULL DEFAULT false;

ALTER TABLE oauth_clients
    ADD CONSTRAINT oauth_clients_impersonation_ck CHECK (
        (NOT may_impersonate OR org_id IS NOT NULL)
        AND (NOT may_impersonate_admins OR may_impersonate)
    );
