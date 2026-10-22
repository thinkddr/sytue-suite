-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- TRAZA DE AUDITORÍA. Espeja el `audit_log` de Drive (mig 0012), Mail y
-- services/id: mismas columnas y mismo aislamiento, para que la Console pueda
-- leer los cuatro con un solo contrato.
--
-- # Qué se audita aquí, y por qué no todo
--
-- No es un registro de actividad: escribir una nota no entra. Entra lo que NO se
-- puede deshacer y lo que amplía quién puede ver algo:
--
--   · `notebook.publish` — pasar un cuaderno a visibilidad `public` le crea un
--     token y lo deja legible SIN SESIÓN por cualquiera que tenga el enlace. Es
--     la única superficie del servicio que sale de la organización, y hoy no
--     queda constancia de quién la abrió ni cuándo.
--   · `notebook.unpublish` — el par de la anterior. Sin ella, un cuaderno que
--     estuvo publicado y dejó de estarlo no se distingue de uno que nunca lo
--     estuvo, y la ventana de exposición no se puede reconstruir.
--   · `note.purge` y `notebook.delete` — destruyen contenido. La papelera tiene
--     vuelta atrás; la purga no.
--
-- Lo demás se deja fuera a propósito: una traza que registra cada tecleo se
-- vuelve ilegible y nadie la mira, que es como no tenerla pero pagando el disco.
--
-- # Aislamiento
--
-- RLS deny-by-default por `app.tenant`, igual que las seis tablas de la 0001. Un
-- administrador NUNCA ve la traza de otra organización. Sin política PERMISSIVE
-- de mantenimiento ni de enlace público: aquí no entra ningún worker
-- cross-tenant, y el cuaderno publicado se lee sin sesión pero no genera ni lee
-- auditoría.

CREATE TABLE audit_log (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    tenant      text        NOT NULL,
    actor       text        NOT NULL,                     -- `sub` de Sytue ID de quien lo hizo
    action      text        NOT NULL,                     -- notebook.publish|notebook.unpublish|notebook.delete|note.purge
    target      text        NOT NULL DEFAULT '',          -- id del cuaderno o de la nota
    detail      jsonb       NOT NULL DEFAULT '{}'::jsonb  -- título, visibilidad anterior y nueva
);

-- La consulta de la Console es siempre «lo último de esta organización».
CREATE INDEX audit_log_tenant_idx ON audit_log (tenant, occurred_at DESC);

ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log FORCE ROW LEVEL SECURITY;

CREATE POLICY audit_log_tenant_isolation ON audit_log
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- FUSIÓN DE ORGANIZACIONES, por el mismo motivo que la 0002 se la dio a las seis
-- tablas de la 0001: al fusionar dos organizaciones se reescribe el `tenant` de
-- cada fila, y bajo FORCE ROW LEVEL SECURITY la política de aislamiento no VE las
-- del origen, así que el UPDATE afectaría a cero filas y la fusión se declararía
-- completada dejando esta tabla atrás.
--
-- La 0002 recorre las tablas existentes en el momento de aplicarse, así que una
-- creada después —esta— no la recibe. Va aquí y no en una migración aparte
-- porque la tabla y su política son la misma decisión, y separarlas es cómo se
-- consigue que la segunda se olvide.
--
-- Mismo alcance estrecho: `app.merge` solo la fija `pg.InMergeTx`, que solo usa
-- el motor de fusión de Sytue ID. Ninguna transacción de negocio la fija, así que
-- el aislamiento de arriba queda igual que antes.
CREATE POLICY audit_log_platform_merge ON audit_log
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
