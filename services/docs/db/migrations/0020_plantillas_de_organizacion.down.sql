-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Bajar esto quita la biblioteca de plantillas de la organización y su registro. Los
-- documentos que estaban marcados y las instantáneas de sus versiones NO se tocan.
DROP TABLE IF EXISTS doc_org_template_log;
DROP TABLE IF EXISTS doc_org_templates;
