-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Puestos de la SUITE Sytue (Mail, Drive, Docs, Rooms) contratados por la
-- organización, a 6 EUR por usuario y mes.
--
-- Sytue Cloud no se vende suelto: el plan FREE exige al menos un puesto. Es decir,
-- «Free» significa «sin coste ADICIONAL sobre tu suscripción a la suite», no
-- gratis a secas — y el producto tiene que decirlo así, porque anunciar un plan
-- gratuito que en realidad requiere pagar 6 EUR/usuario sería exactamente el mismo
-- problema que los precios inventados del dashboard.
--
-- DEFAULT 0 a propósito: no se le regala una suscripción a nadie. Las
-- organizaciones existentes quedan sin puestos y, si están en Free, el guardia las
-- frena hasta que se registre su suscripción. Es el estado honesto, no el cómodo.

ALTER TABLE organizations
    ADD COLUMN suite_seats integer NOT NULL DEFAULT 0;

ALTER TABLE organizations
    ADD CONSTRAINT organizations_suite_seats_nonneg CHECK (suite_seats >= 0);
