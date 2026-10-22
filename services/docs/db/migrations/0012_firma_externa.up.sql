-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- «Pedir firma» a alguien de OTRA organización (docs/decisions.md#0057).
--
-- La petición vive en la organización de quien pide, como el documento del 0052 y la
-- conversación del 0040: quien firma es una fila con `externo`. El PDF viaja por lo
-- que Drive ya tiene para sacar algo fuera, un ENLACE de solo lectura, que crea este
-- servicio con la sesión de quien pide y que nadie más ve:
--
-- · `enlace`: el token del enlace, CIFRADO con la clave del servicio. Primero el del
--   original (con él quien firma se trae su copia, y otra vez si la borra); al firmar,
--   el de la copia firmada (con él quien pidió se la trae). Se borra al traer el
--   firmado y al cerrarse la petición sin firmar: pasado ese momento nadie lo conoce.
-- · `enlace_share`: el id del enlace del ORIGINAL en Drive, para revocarlo con la
--   sesión de quien pidió en cuanto deja de hacer falta. Vacío = ya revocado.
-- · `copia`, `copia_ruta`: la copia del original en la unidad de quien firma. Es la
--   que abre, la que firma y contra la que se comprueba lo que entrega.
-- · `huella`: SHA-256 del original al pedir. La copia que se trae quien firma tiene
--   que ser ESE fichero, y lo que firma, esa copia sin tocar.
-- · `firmado_huella`: SHA-256 de lo entregado y comprobado. Lo que se trae quien pidió
--   tiene que ser exactamente eso, aunque el fichero cambie después en la otra unidad.
-- · `ocupada`: una vuelta de «abrir» o «traer» en marcha (dos a la vez dejarían dos
--   copias); caduca sola a los dos minutos si la vuelta muere a medias.
ALTER TABLE pdf_peticiones_firma
    ADD COLUMN externo        boolean NOT NULL DEFAULT false,
    ADD COLUMN enlace         bytea,
    ADD COLUMN enlace_share   text,
    ADD COLUMN copia          text,
    ADD COLUMN copia_ruta     text,
    ADD COLUMN huella         text,
    ADD COLUMN firmado_huella text,
    ADD COLUMN ocupada        timestamptz,
    ADD CONSTRAINT pdf_peticiones_firma_externo_chk
        CHECK (externo OR (enlace IS NULL AND enlace_share IS NULL AND copia IS NULL
                           AND copia_ruta IS NULL AND huella IS NULL AND firmado_huella IS NULL)),
    ADD CONSTRAINT pdf_peticiones_firma_externo_huella_chk
        CHECK (NOT externo OR huella IS NOT NULL),
    -- Cerrada sin firmar, no queda ningún token.
    ADD CONSTRAINT pdf_peticiones_firma_enlace_cerrada_chk
        CHECK (estado NOT IN ('rechazada', 'cancelada') OR enlace IS NULL);

CREATE INDEX pdf_peticiones_firma_externo_idx
    ON pdf_peticiones_firma (destinatario, creada DESC) WHERE externo;

-- La organización anfitriona de una petición, SOLO para su destinatario de fuera.
-- No devuelve nada más que el slug.
CREATE FUNCTION docs_firma_anfitrion(p_id uuid, p_sub text)
RETURNS text
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
STABLE
AS $$
    SELECT tenant FROM pdf_peticiones_firma
    WHERE id = p_id AND destinatario = p_sub AND externo
$$;

-- Las que le han pedido a `p_sub` desde otras organizaciones, para su lista. Sin la
-- ruta del original ni su id en la anfitriona (dirían cómo organiza sus carpetas
-- otra organización): solo el NOMBRE del fichero y, si ya la trajo, SU copia. Ni el
-- enlace, claro.
CREATE FUNCTION docs_firmas_de_fuera(p_sub text, p_tenant text)
RETURNS TABLE (id uuid, solicitante text, solicitante_nombre text, destinatario text,
               destinatario_correo text, nombre text, copia text, copia_ruta text,
               mensaje text, estado text, creada timestamptz, resuelta timestamptz)
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
STABLE
AS $$
    SELECT id, solicitante, solicitante_nombre, destinatario, destinatario_correo,
           substring(ruta from '[^/]*$'), coalesce(copia, ''), coalesce(copia_ruta, ''),
           mensaje, estado, creada, resuelta
    FROM pdf_peticiones_firma
    WHERE destinatario = p_sub AND externo AND tenant <> p_tenant
    ORDER BY creada DESC
    LIMIT 200
$$;

-- Cuántas pendientes de fuera tiene `p_sub`, de todas las organizaciones: el tope que
-- impide que nadie le llene la lista (y la unidad) a alguien de otra organización.
CREATE FUNCTION docs_firmas_pendientes_para(p_sub text)
RETURNS bigint
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
STABLE
AS $$
    SELECT count(*) FROM pdf_peticiones_firma
    WHERE destinatario = p_sub AND externo AND estado = 'pendiente'
$$;

REVOKE ALL ON FUNCTION docs_firmas_pendientes_para(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION docs_firmas_pendientes_para(text) TO docs_app;
REVOKE ALL ON FUNCTION docs_firma_anfitrion(uuid, text) FROM PUBLIC;
REVOKE ALL ON FUNCTION docs_firmas_de_fuera(text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION docs_firma_anfitrion(uuid, text) TO docs_app;
GRANT EXECUTE ON FUNCTION docs_firmas_de_fuera(text, text) TO docs_app;
