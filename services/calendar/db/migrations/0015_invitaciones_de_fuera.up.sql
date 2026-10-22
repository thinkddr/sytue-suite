-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- LAS INVITACIONES DE OTRA ORGANIZACIÓN SE VEN DESDE LA PROPIA (COM-15, decidido por el
-- dueño el 2026-10-17). Una invitación vive en la organización que convoca; quien la
-- recibe desde otra la busca desde la suya, siempre por SUS direcciones personales y su
-- `sub`, del token verificado (store/invitaciones_de_fuera.go). Solo lectura: responder
-- se hace dentro de la anfitriona, con su RLS. Igual que Rooms (mig 0041).
CREATE POLICY invitations_de_fuera ON invitations FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on');
