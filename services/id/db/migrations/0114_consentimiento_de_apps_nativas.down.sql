-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Al revertir, las apps nativas vuelven a recibir el código sin preguntar (NAT-02). Los
-- tres redirects huérfanos NO se devuelven: no los usa ninguna app.
DROP TABLE IF EXISTS consentimientos_nativos;
ALTER TABLE auth_codes DROP COLUMN IF EXISTS dispositivo;
