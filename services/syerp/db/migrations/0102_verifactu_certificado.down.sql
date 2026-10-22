-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
ALTER TABLE organizacion
    DROP COLUMN verifactu_sello,
    DROP COLUMN verifactu_cert_nif,
    DROP COLUMN verifactu_key_enc,
    DROP COLUMN verifactu_cert_pem;
