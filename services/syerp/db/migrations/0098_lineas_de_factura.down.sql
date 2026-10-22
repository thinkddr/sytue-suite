-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP TRIGGER IF EXISTS records_linea_de_factura_emitida ON records;
DROP TRIGGER IF EXISTS records_linea_de_factura_nueva ON records;
DROP FUNCTION IF EXISTS verifactu_linea_de_factura_emitida();
DROP FUNCTION IF EXISTS verifactu_factura_cerrada(text, text);

-- La cabecera vuelve a ser la de la 0010.
CREATE OR REPLACE FUNCTION verifactu_factura_emitida() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    ultimo_tipo   text;
    ultimo_estado text;
BEGIN
    SELECT tipo, estado INTO ultimo_tipo, ultimo_estado
      FROM verifactu_registros
     WHERE tenant = OLD.tenant AND factura = OLD.id
     ORDER BY secuencia DESC
     LIMIT 1;
    IF NOT FOUND THEN
        RETURN NULL;
    END IF;
    IF TG_OP = 'DELETE'
       OR NEW.estado = 'borrador'
       OR (OLD.estado = 'anulado' AND NEW.estado <> 'anulado')
       OR (NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL)
       OR NEW.ref IS DISTINCT FROM OLD.ref THEN
        RAISE EXCEPTION 'la factura ya se ha enviado a la Agencia Tributaria' USING ERRCODE = 'SY002';
    END IF;
    IF NEW.estado = 'anulado' AND OLD.estado <> 'anulado' AND ultimo_tipo <> 'anulacion' THEN
        RAISE EXCEPTION 'la factura se anula con su registro de anulación' USING ERRCODE = 'SY002';
    END IF;
    IF NOT (ultimo_tipo = 'alta' AND ultimo_estado = 'incorrecto')
       AND (NEW.data->'fecha', NEW.data->'cliente', NEW.data->'base', NEW.data->'iva', NEW.data->'total',
            NEW.data->'regimen_iva', NEW.data->'rectifica')
           IS DISTINCT FROM
           (OLD.data->'fecha', OLD.data->'cliente', OLD.data->'base', OLD.data->'iva', OLD.data->'total',
            OLD.data->'regimen_iva', OLD.data->'rectifica') THEN
        RAISE EXCEPTION 'la factura ya se ha enviado a la Agencia Tributaria' USING ERRCODE = 'SY002';
    END IF;
    RETURN NULL;
END
$$;
