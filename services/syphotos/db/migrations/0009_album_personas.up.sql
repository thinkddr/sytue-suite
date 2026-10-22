-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- ÁLBUM COMPARTIDO CON UNA PERSONA de la organización, por su `sub`.
--
-- Al lado del enlace (0004) y no en su lugar: el enlace lo abre quien lo tenga, sin
-- cuenta; esto lo abre SOLO la persona a quien se le dio, con su sesión, y se retira
-- persona a persona. Hoy es solo para ver: quien lo recibe no añade ni quita fotos.
CREATE TABLE album_personas (
    album_id      uuid        NOT NULL REFERENCES albums (id) ON DELETE CASCADE,
    tenant        text        NOT NULL,
    sujeto        text        NOT NULL,
    concedido_por text        NOT NULL,
    creado        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (album_id, sujeto)
);

-- «Compartido conmigo» busca por persona, no por álbum.
CREATE INDEX album_personas_sujeto ON album_personas (tenant, sujeto);

ALTER TABLE album_personas ENABLE ROW LEVEL SECURITY;
ALTER TABLE album_personas FORCE ROW LEVEL SECURITY;
CREATE POLICY album_personas_tenant_isolation ON album_personas
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY album_personas_platform_merge ON album_personas
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
