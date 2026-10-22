-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte 0001_init: elimina las tablas de Sytue Hub y sus políticas RLS.

DROP TABLE IF EXISTS presence;
DROP TABLE IF EXISTS notifications;
