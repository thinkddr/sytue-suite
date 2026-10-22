-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Política de fusión para las tablas de S/MIME (0011).
--
-- La 0010 barrió las tablas que existían ENTONCES. Estas dos nacieron después,
-- así que se quedaron fuera y la primera fusión de organizaciones abortó en el
-- preflight con «sin política de fusión en smime_authorities, smime_identities».
--
-- Es la mitad barata del arreglo: la otra es el gate de platform/pkg/migcheck,
-- que a partir de ahora rechaza una migración que cree una tabla con columna
-- `tenant` sin su política, para que la tercera vez no exista.
--
-- Sobre el material criptográfico: mover estas filas NO las descifra ni las
-- reexpone. `key_enc` sigue cifrada con la KEK del servicio, que es del servicio
-- y no de la organización, así que el destino puede leerla exactamente igual que
-- el origen. Lo que cambia es de quién son.

DROP POLICY IF EXISTS smime_authorities_platform_merge ON smime_authorities;
CREATE POLICY smime_authorities_platform_merge ON smime_authorities
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

DROP POLICY IF EXISTS smime_identities_platform_merge ON smime_identities;
CREATE POLICY smime_identities_platform_merge ON smime_identities
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
