-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Logs de runtime BUSCABLES + su retención (WS4 / Ola O1).
--
-- Hasta ahora los logs de una app solo se TRANSMITÍAN: el SSE de
-- /projects/{id}/deployments/{d}/logs abre un tail contra el pod y lo que no
-- estuvieras mirando en ese instante se perdía para siempre. «Buscar en los
-- logs» exige que estén escritos en algún sitio; esta tabla es ese sitio.
--
-- Por qué en PostgreSQL y no en el Object Storage donde ya viven los logs de
-- BUILD: un log de build se lee entero y de una vez (una clave → un objeto, y
-- eso ya funciona), mientras que los de runtime se consultan por trozos —«los
-- errores de ayer entre las 15:00 y las 16:00»— y eso es un índice por
-- (proyecto, tiempo) que S3 no tiene. Lo que acota el coste no es el motor sino
-- la RETENCIÓN: 14 días, purgados por el mismo cron de mantenimiento que el
-- resto (store.RuntimeLogRetention, política PERMISSIVE de abajo).

CREATE TABLE runtime_logs (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant        text        NOT NULL,
    project_id    uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    -- CASCADE a propósito: al purgar un preview retirado (7 días) sus logs se
    -- van con él sin que el cron tenga que saber de esta tabla.
    deployment_id uuid        NOT NULL REFERENCES deployments (id) ON DELETE CASCADE,
    -- ts es el instante que puso el CONTENEDOR (kubelet), no el de la ingesta:
    -- si el recolector se retrasa, la búsqueda por tiempo sigue siendo honesta.
    ts            timestamptz NOT NULL,
    line          text        NOT NULL
);

-- Índice de la búsqueda: (tenant, project_id) acota, ts DESC ordena y pagina.
-- El desempate por id lo exige la paginación KEYSET (CONVENTIONS §0.8): el
-- predicado (ts, id) < ($ts, $id) recorre este índice sin re-ordenar ni OFFSET.
CREATE INDEX runtime_logs_search_idx ON runtime_logs (tenant, project_id, ts DESC, id DESC);
-- Purga por antigüedad (Index Scan, no Seq Scan; mismo criterio que 0002).
CREATE INDEX runtime_logs_ts_idx ON runtime_logs (ts);

ALTER TABLE runtime_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE runtime_logs FORCE ROW LEVEL SECURITY;

-- Aislamiento por organización. Los logs de producción contienen secretos y PII:
-- es la tabla donde MENOS puede fallar (por eso también FORCE, que aplica la
-- política incluso al dueño de la tabla).
CREATE POLICY runtime_logs_tenant ON runtime_logs
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Mantenimiento: la purga por retención recorre TODOS los tenants. Mismo patrón
-- exacto que 0002/0010/0022 (`app.maintenance='on'`, que solo fija
-- pg.InMaintenanceTx) — nunca un rol con BYPASSRLS.
CREATE POLICY runtime_logs_maintenance ON runtime_logs
    USING (current_setting('app.maintenance', true) = 'on');

-- Estado del recolector, en el propio deployment para no crear una tabla de una
-- sola fila por deployment. Son DOS relojes con semánticas distintas:
--
--   * logs_collected_at es el CORTE de lectura: el timestamp de la última línea
--     persistida, que se entrega como `since` a Kubernetes.
--   * logs_collect_claimed_at es el ARRIENDO: el instante en que una réplica
--     reclamó el deployment, para que otra no lea el mismo pod a la vez.
--
-- No se pueden mezclar: adelantar el arriendo a `now` haría que un lote cuyas
-- líneas sean anteriores a `now` no pudiera volver a dejar el cursor en la
-- última línea real. NULL significa nunca recolectado/reclamado, respectivamente.
ALTER TABLE deployments ADD COLUMN logs_collected_at timestamptz;
ALTER TABLE deployments ADD COLUMN logs_collect_claimed_at timestamptz;

-- Cola del recolector: solo deployments vivos, los más atrasados primero. El
-- NULLS FIRST del índice tiene que coincidir con el del ORDER BY de la consulta
-- (el defecto en ASC es NULLS LAST) o el planificador no lo usaría.
CREATE INDEX deployments_logs_collect_idx
    ON deployments (logs_collect_claimed_at NULLS FIRST) WHERE status = 'ready';
