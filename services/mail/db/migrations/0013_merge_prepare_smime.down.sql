-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira el gancho. Sin él la fusión vuelve a abortar cuando las dos
-- organizaciones tienen autoridad S/MIME, pero no rompe nada más: el motor solo
-- lo llama si existe.

DROP FUNCTION IF EXISTS platform_merge_prepare(text, text);
