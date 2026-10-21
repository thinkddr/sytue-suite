-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- La forma por defecto de la organización (docs/design.md, «forma-marco»).
--
-- La forma es el tercer eje de la apariencia, junto al modo y al color: «Marco», la
-- de la casa, o «Carril». Cada persona elige la suya; quien no ha elegido ve la de
-- su organización, que fija el administrador en Sytue Admin. Como con el color
-- (0107), aquí se guarda ese valor por defecto y no la elección de nadie.
--
-- En `organizations` y no en `org_branding`, por lo mismo que el color: la fila la
-- lee ya el emisor de tokens y el claim `org_forma` no cuesta una consulta más.
--
-- LA LISTA DE FORMAS VIVE EN EL CÓDIGO (domain.OrgFormas): el handler valida contra
-- ella antes de escribir. Aquí solo se garantiza la FORMA del valor, porque añadir
-- una no puede pedir reescribir un CHECK, y una migración aplicada no se reescribe.
ALTER TABLE organizations
    ADD COLUMN forma text NOT NULL DEFAULT 'marco' CHECK (forma ~ '^[a-z]{1,16}$');

COMMENT ON COLUMN organizations.forma IS
    'Forma por defecto de la suite para quien no ha elegido la suya. Valores en domain.OrgFormas.';
