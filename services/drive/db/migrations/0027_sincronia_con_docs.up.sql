-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- LA PAPELERA DE DRIVE Y LA DE DOCS SON UNA (decidido por el dueño, 2026-09-28).
-- Un documento de Sytue Docs vive como su .odt/.ods/.odp en la unidad de su dueño
-- (services/docs/internal/http/hogar_en_drive.go); mandar ese fichero a la papelera,
-- sacarlo de ella o purgarlo tiene que hacer lo mismo con el documento.
--
-- POR QUÉ UN DISPARADOR Y NO UNA LLAMADA DESDE CADA RUTA. Un fichero entra y sale
-- de la papelera por muchos caminos: uno a uno, en lote, con la carpeta que lo
-- contiene (cascada), desde una unidad de equipo y por la auto-purga del worker.
-- Engancharse a cada uno es dejar fuera el siguiente que alguien escriba. El
-- disparador ve TODOS, y en la MISMA transacción que el cambio: si el cambio se
-- deshace, la marca también.
--
-- LA COLA NO GUARDA EL ESTADO, solo QUÉ fichero cambió. Quien la vacía
-- (internal/docssync) lee el estado ACTUAL del fichero al entregarlo: dos cambios
-- seguidos se funden en una fila y lo que llega a Docs es siempre lo último, sin
-- depender del orden de entrega. owner_sub y name se guardan porque, si el fichero
-- se purga, ya no hay fila de la que leerlos.
CREATE TABLE docs_sync_pending (
    tenant      text        NOT NULL,
    file_id     uuid        NOT NULL,
    owner_sub   text        NOT NULL DEFAULT '',
    name        text        NOT NULL DEFAULT '',
    enqueued_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    -- Entregas fallidas seguidas: la siguiente se aplaza (enqueued_at) cada vez más,
    -- para que una organización que falla no tape a las demás ni martillee a Docs.
    attempts    integer     NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant, file_id)
);

CREATE INDEX docs_sync_pending_enqueued_idx ON docs_sync_pending (enqueued_at);

ALTER TABLE docs_sync_pending ENABLE ROW LEVEL SECURITY;
ALTER TABLE docs_sync_pending FORCE ROW LEVEL SECURITY;

CREATE POLICY docs_sync_pending_tenant_isolation ON docs_sync_pending
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- El worker la vacía para todas las organizaciones (como la auto-purga, 0009).
CREATE POLICY docs_sync_pending_maintenance ON docs_sync_pending
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

-- ENCOLAR SE PUEDE SIEMPRE, sea cual sea el contexto de RLS de quien toca `files`.
-- No abre nada: la fila solo dice «mira este fichero», y el worker entrega el
-- estado que lee de `files`, no lo que diga la fila. Así el disparador no pierde un
-- cambio por venir de una ruta sin `app.tenant`.
CREATE POLICY docs_sync_pending_encolar ON docs_sync_pending
    FOR INSERT WITH CHECK (true);

CREATE POLICY docs_sync_pending_platform_merge ON docs_sync_pending
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

-- Solo los formatos en los que nace un documento de Docs, y solo en «Mi unidad»,
-- que es donde Docs los deja: el resto de ficheros no tiene documento que acompañar
-- y no llena la cola. Las unidades de equipo quedan fuera en los DOS sentidos (Docs
-- tampoco las alcanza: su RLS pide `app.team_drive`), y los adjuntos de canal
-- también.
--
-- NUNCA TUMBA LA OPERACIÓN DE DRIVE. El INSERT lo permite siempre la política de
-- arriba; lo único que podría fallar es el UPDATE del ON CONFLICT en una ruta sin
-- contexto de RLS, que hoy no existe. Si llegara a pasar se pierde la marca, con un
-- WARNING en el registro de PostgreSQL, y no el borrado de la persona.
CREATE FUNCTION drive_docs_sync_encolar() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    f files%ROWTYPE;
BEGIN
    IF TG_OP = 'DELETE' THEN
        f := OLD;
    ELSE
        f := NEW;
    END IF;
    IF f.channel_id IS NOT NULL OR f.team_drive_id IS NOT NULL
       OR lower(f.name) !~ '\.(odt|ods|odp)$' THEN
        RETURN NULL;
    END IF;
    BEGIN
        INSERT INTO docs_sync_pending (tenant, file_id, owner_sub, name)
        VALUES (f.tenant, f.id, coalesce(f.owner_sub, ''), f.name)
        ON CONFLICT (tenant, file_id) DO UPDATE
            SET owner_sub = EXCLUDED.owner_sub, name = EXCLUDED.name,
                enqueued_at = EXCLUDED.enqueued_at, attempts = 0;
    EXCEPTION WHEN insufficient_privilege THEN
        RAISE WARNING 'drive: sin contexto RLS para encolar % a Docs', f.id;
    END;
    RETURN NULL;
END;
$$;

CREATE TRIGGER files_docs_sync_papelera
    AFTER UPDATE OF deleted_at ON files
    FOR EACH ROW
    WHEN (OLD.deleted_at IS DISTINCT FROM NEW.deleted_at)
    EXECUTE FUNCTION drive_docs_sync_encolar();

CREATE TRIGGER files_docs_sync_purga
    AFTER DELETE ON files
    FOR EACH ROW
    EXECUTE FUNCTION drive_docs_sync_encolar();
