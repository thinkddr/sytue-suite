-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- UNFURL de enlaces (Ola 16 · paridad Mattermost): caché de metadatos OpenGraph
-- de las URLs http/https que aparecen en los mensajes. Cuando un cliente pide la
-- vista previa de una URL (GET /unfurl?url=), el servicio la obtiene UNA vez con
-- el cliente SSRF-safe (Ola 15) y cachea aquí el resultado; peticiones posteriores
-- —del mismo tenant o de otro usuario— reusan la fila sin re-fetch. Un TTL en el
-- servicio (fetched_at) fuerza el refresco periódico.
--
-- ESCALA: la PK (tenant, url_hash) es a la vez el índice que cubre el lookup por
-- tenant+hash (sha256 hex de la URL, longitud fija) sin escaneo ni índice extra; a
-- cientos de miles de usuarios el fetch remoto ocurre como mucho una vez por URL y
-- tenant por ventana de TTL. Una fila con title/description/image vacíos es un
-- resultado NEGATIVO cacheado (la URL no dio metadatos): evita re-fetchear lo que
-- ya sabemos que no tiene tarjeta.
--
-- RLS deny-by-default por tenant (mismo patrón que custom_emoji / read_state). Los
-- metadatos son de URLs públicas, pero se cachean por tenant para no filtrar entre
-- inquilinos qué enlaces se han visitado.

CREATE TABLE link_unfurls (
    tenant      text        NOT NULL,
    url_hash    text        NOT NULL,   -- sha256 hex de la URL (clave estable, long. fija)
    url         text        NOT NULL,
    title       text        NOT NULL DEFAULT '',
    description text        NOT NULL DEFAULT '',
    image_url   text        NOT NULL DEFAULT '',
    site_name   text        NOT NULL DEFAULT '',
    fetched_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, url_hash)
);

ALTER TABLE link_unfurls ENABLE ROW LEVEL SECURITY;
ALTER TABLE link_unfurls FORCE ROW LEVEL SECURITY;
CREATE POLICY link_unfurls_tenant_isolation ON link_unfurls
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
