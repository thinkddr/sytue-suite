-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte el esquema base de Sytue Chat.

DROP TABLE IF EXISTS events_outbox;
DROP TABLE IF EXISTS read_state;
DROP TABLE IF EXISTS reactions;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS channel_members;
DROP TABLE IF EXISTS channels;
DROP TABLE IF EXISTS team_members;
DROP TABLE IF EXISTS teams;
