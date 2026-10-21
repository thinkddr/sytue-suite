-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Pages registra los hosts bajo sites.<dominio> por una ruta interna autenticada.
-- Puede reasignarlos al tenant que afirma Pages: sin esta política, RLS solo vería
-- el dueño anterior y ON CONFLICT no podría corregir una apropiación previa.
-- El flag se fija LOCAL únicamente en Store.EnsureSite tras SoloDesdeDentro.
CREATE POLICY analytics_sites_pages_hosts ON analytics_sites
    USING (current_setting('app.pages_hosts', true) = 'on')
    WITH CHECK (current_setting('app.pages_hosts', true) = 'on');
