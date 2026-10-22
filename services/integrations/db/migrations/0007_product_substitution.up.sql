-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Sustitución de producto: la libertad de elección de la suite.
--
-- Una organización puede decidir que un producto Sytue lo sirva una aplicación
-- conectada: Google Sheets en vez de Sytue Sheets, Teams en vez de Rooms, Azure en vez
-- de Cloud. La fila dice «para ESTE producto, en ESTA organización, abre ESTA conexión».
--
-- Una fila POR PRODUCTO (clave primaria compuesta): sustituir es elegir, no acumular. Si
-- se pudieran tener dos sustitutos de Sytue Sheets, el lanzador tendría que desempatar y
-- la respuesta a «¿dónde se abre esto?» dejaría de ser única.
--
-- La ausencia de fila es el estado por defecto y significa «el producto Sytue». No hay
-- fila que diga «usa el nativo»: el nativo es lo que hay cuando nadie ha elegido otra
-- cosa, y modelarlo con una fila obligaría a sembrar una por producto y organización.
CREATE TABLE product_substitutions (
    org_id      text        NOT NULL,
    -- product es la clave canónica del producto (mail, drive, sheets…), la misma que
    -- ProductKey en el front. Sin FK: el catálogo de productos vive en el código, no en
    -- una tabla, y duplicarlo aquí crearía dos fuentes de verdad que se desincronizan.
    product     text        NOT NULL,
    -- ON DELETE CASCADE: si se borra la conexión, la sustitución desaparece con ella y
    -- el producto vuelve al nativo. Lo contrario dejaría al lanzador apuntando a una
    -- conexión que ya no existe.
    instance_id uuid        NOT NULL REFERENCES app_instances (id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, product)
);

-- Resolver «¿qué sustituye a este producto?» va por la clave primaria. Este índice sirve
-- al camino contrario, que es el que usa la baja: al desconectar una app hay que saber
-- qué productos dejaba servidos para avisar de que vuelven al nativo.
CREATE INDEX product_substitutions_instance_idx ON product_substitutions (instance_id);

ALTER TABLE product_substitutions ENABLE ROW LEVEL SECURITY;
ALTER TABLE product_substitutions FORCE ROW LEVEL SECURITY;

CREATE POLICY product_substitutions_org_isolation ON product_substitutions
    USING (org_id = current_setting('app.tenant', true))
    WITH CHECK (org_id = current_setting('app.tenant', true));

GRANT SELECT, INSERT, UPDATE, DELETE ON product_substitutions TO integrations_app;
