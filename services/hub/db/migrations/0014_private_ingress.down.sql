-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- La entrada privada guarda texto cifrado, recibos y autorizaciones firmadas, y la
-- copia de los avisos sin confirmar: bajar la borraría. Como en Mail y Calendar, no
-- se baja sin una vuelta atrás que los conserve.
DO $$ BEGIN RAISE EXCEPTION 'hub private ingress requires a verified notice-preserving rollback' USING ERRCODE = '55000'; END $$;
