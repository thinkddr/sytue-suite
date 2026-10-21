-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE mail_filters      NO FORCE ROW LEVEL SECURITY;
ALTER TABLE mail_labels       NO FORCE ROW LEVEL SECURITY;
ALTER TABLE mail_vacation     NO FORCE ROW LEVEL SECURITY;
ALTER TABLE mail_vacation_log NO FORCE ROW LEVEL SECURITY;
