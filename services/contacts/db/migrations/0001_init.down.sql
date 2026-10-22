-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Reversion del esquema base de Sytue Contacts. El orden respeta las
-- dependencias de claves foraneas (hijos antes que padres).

DROP TABLE IF EXISTS contact_group_members;
DROP TABLE IF EXISTS contact_groups;
DROP TABLE IF EXISTS contact_phones;
DROP TABLE IF EXISTS contact_emails;

DROP TRIGGER IF EXISTS contacts_search_trg ON contacts;
DROP FUNCTION IF EXISTS contacts_search_update();
DROP TABLE IF EXISTS contacts;

DROP TABLE IF EXISTS card_objects;
DROP TABLE IF EXISTS address_books;
DROP TABLE IF EXISTS events_outbox;
