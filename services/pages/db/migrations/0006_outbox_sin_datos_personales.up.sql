-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- WEB-10 (auditoría de seguridad 2026-10-06). El outbox de Sytue Web, sin RLS y sin
-- purga, guardaba el correo y el nombre del comprador, la factura (NIF, nombre,
-- dirección) y las líneas de cada pedido cobrado, los campos de cada formulario con a
-- quién avisar, y el correo de cada desistimiento. Desde este cambio solo lleva
-- identificadores; esto quita lo que ya hubiera.
UPDATE events_outbox
   SET payload = payload - ARRAY['buyer_email', 'buyer_name', 'invoice', 'items', 'fields', 'notify', 'email']
 WHERE payload ?| ARRAY['buyer_email', 'buyer_name', 'invoice', 'items', 'fields', 'notify', 'email'];
