-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte el logotipo de organización. Se pierden los logos subidos —el original
-- no se guardaba en ninguna parte, así que hay que volver a subirlos—; la suite
-- vuelve a enseñar la marca de Sytue teñida por slug, que no necesita esta tabla.
DROP TABLE IF EXISTS org_branding;
