-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Certificado cualificado propio de la organización para VERI*FACTU (NEG-08).
-- En producción cada organización presenta con su propio certificado, cuyo NIF
-- de titular o entidad representada tiene que coincidir con el NIF emisor.
ALTER TABLE organizacion
    ADD COLUMN verifactu_cert_pem text    NOT NULL DEFAULT '',
    ADD COLUMN verifactu_key_enc  text    NOT NULL DEFAULT '',
    ADD COLUMN verifactu_cert_nif text    NOT NULL DEFAULT '',
    ADD COLUMN verifactu_sello    boolean NOT NULL DEFAULT false;
