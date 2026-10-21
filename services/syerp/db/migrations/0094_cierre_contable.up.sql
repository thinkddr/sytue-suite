-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- PERÍODOS CONTABLES: cerrar el libro y que quede cerrado.
--
-- El libro diario ya era inmutable apunte a apunte (0010: un asiento no se
-- edita, se anula con otro), pero no tenía FECHA DE CORTE: se podía asentar en
-- marzo del año pasado —después de presentar el modelo— y el balance de un
-- ejercicio ya declarado cambiaba solo. Es el fallo contable clásico y no lo
-- evita ninguna regla de inmutabilidad por asiento, porque cada asiento nuevo
-- es, por separado, perfectamente válido.
--
-- POR QUÉ UNA FECHA Y NO UNA TABLA DE PERÍODOS. Lo que hace falta responder es
-- «¿puedo asentar con fecha X?», y eso es una comparación con la última fecha
-- cerrada. Una tabla de períodos con sus estados sería el mismo dato repartido
-- en doce filas al año, con la pregunta «¿y los huecos?» encima. Cuando haga
-- falta cerrar un mes de en medio dejando abierto el anterior, esta tabla se
-- amplía; hoy nadie cierra así.
--
-- APPEND-ONLY. Cada cierre y cada REAPERTURA dejan su fila con motivo y autor:
-- reabrir un ejercicio cerrado es excepcional y justo por eso tiene que verse.
-- El estado de ahora es la última fila; el historial es la tabla entera.

CREATE TABLE cierre_contable (
    id      uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant  text        NOT NULL,
    -- Última fecha CERRADA: no se puede asentar ni anular con fecha <= hasta.
    -- NULL solo en una reapertura total (el libro entero vuelve a estar abierto).
    hasta   date,
    accion  text        NOT NULL,
    motivo  text        NOT NULL DEFAULT '',
    por     text        NOT NULL DEFAULT '',
    en      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT cierre_contable_accion CHECK (accion IN ('cierre', 'reapertura')),
    CONSTRAINT cierre_contable_motivo CHECK (length(motivo) <= 1000),
    -- Un cierre sin fecha no cierra nada: sería una fila que no dice nada.
    CONSTRAINT cierre_contable_hasta CHECK (accion = 'reapertura' OR hasta IS NOT NULL)
);

CREATE INDEX cierre_contable_ultimo ON cierre_contable (tenant, en DESC);

ALTER TABLE cierre_contable ENABLE ROW LEVEL SECURITY;
ALTER TABLE cierre_contable FORCE ROW LEVEL SECURITY;

CREATE POLICY cierre_contable_tenant_isolation ON cierre_contable
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Literal, como en la 0009 y la 0011: el gate de fusión lee el SQL sin ejecutarlo.
CREATE POLICY cierre_contable_platform_merge ON cierre_contable
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
