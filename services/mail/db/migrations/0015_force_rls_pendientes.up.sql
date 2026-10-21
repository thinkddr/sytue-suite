-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- FORCE, no solo ENABLE. Ver services/cloud/db/migrations/0041 para el porqué:
-- `ENABLE ROW LEVEL SECURITY` NO se aplica al dueño de la tabla. Aquí hay
-- filtros, etiquetas y respuestas automáticas de correo: quién escribe a quién y
-- cuándo está fuera. Es de lo más sensible que guarda la plataforma.
ALTER TABLE mail_filters      FORCE ROW LEVEL SECURITY;
ALTER TABLE mail_labels       FORCE ROW LEVEL SECURITY;
ALTER TABLE mail_vacation     FORCE ROW LEVEL SECURITY;
ALTER TABLE mail_vacation_log FORCE ROW LEVEL SECURITY;
