-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira las políticas de fusión de S/MIME. El aislamiento por tenant no depende
-- de ellas: lo aplican smime_authorities_tenant y smime_identities_tenant (0011).

DROP POLICY IF EXISTS smime_authorities_platform_merge ON smime_authorities;
DROP POLICY IF EXISTS smime_identities_platform_merge ON smime_identities;
