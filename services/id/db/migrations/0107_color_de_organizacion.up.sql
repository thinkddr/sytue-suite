-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El color por defecto de la organización (ADR 0071).
--
-- La suite ofrece varios colores y cada persona elige el suyo; quien no ha elegido
-- ve el de su organización, que fija el administrador en Sytue Admin. Lo que se
-- guarda aquí es ese valor por defecto, no la elección de nadie: la de cada
-- persona vive en su navegador, junto a la del modo claro u oscuro.
--
-- Una columna en `organizations` y no en `org_branding`: esa tabla es «sin fila =
-- sin logo» y su fila exige el PNG, así que una organización sin logo propio no
-- tendría dónde guardar su color. Y va en la fila que el emisor de tokens ya lee
-- en cada emisión, de modo que el claim `org_color` no cuesta una consulta más.
--
-- LA LISTA DE COLORES VIVE EN EL CÓDIGO (domain.OrgColors), como el catálogo de
-- permisos de la 0019: el handler valida contra ella antes de escribir. Aquí solo
-- se garantiza la FORMA del valor, porque añadir un color no puede pedir reescribir
-- un CHECK, y una migración aplicada no se reescribe.
ALTER TABLE organizations
    ADD COLUMN color text NOT NULL DEFAULT 'petroleo' CHECK (color ~ '^[a-z]{1,16}$');

COMMENT ON COLUMN organizations.color IS
    'Color por defecto de la suite para quien no ha elegido el suyo. Valores en domain.OrgColors.';
