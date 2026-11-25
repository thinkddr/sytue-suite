-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- LA TRAZA DE ACCESO A DATOS DE CLIENTE (docs/security.md §1.7): quién descargó,
-- abrió, compartió o exportó qué, cuándo y desde dónde, sin el contenido. La
-- escribe platform/pkg/auditlog desde el router del servicio; la tabla y sus
-- garantías (RLS, fusión, retención de acceso.* a doce meses, solo de añadir)
-- son las comunes de platform/pkg/pg/audit_log_v1.sql:
-- sytue:audit-log-v1
--
-- Si db.Migrations no expandiera el marcador, esto falla en vez de dejar el
-- servicio sin traza en silencio.
SELECT 'audit_log'::regclass;

-- Sytue Docs entra en su base con el rol docs_app (store.go, SET LOCAL ROLE).
GRANT SELECT, INSERT ON audit_log TO docs_app;
