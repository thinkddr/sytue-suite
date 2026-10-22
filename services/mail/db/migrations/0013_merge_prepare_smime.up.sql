-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- QUÉ PASA CON EL S/MIME CUANDO DOS ORGANIZACIONES SE FUSIONAN.
--
-- `smime_authorities` tiene PRIMARY KEY (tenant): una autoridad por organización.
-- Si las dos partes de una fusión tienen la suya, el `UPDATE tenant` del motor
-- choca con la clave primaria y aborta la fusión entera. Y aunque no chocara,
-- mover la autoridad del origen no arreglaría nada: los dispositivos de la
-- organización resultante tienen instalado el ancla del DESTINO, así que los
-- certificados firmados por la autoridad del origen dejarían de validar en el
-- momento en que la organización desaparece. Un correo firmado por una autoridad
-- que ya no existe es peor que un correo sin firmar: el cliente lo marca como no
-- confiable en vez de neutro.
--
-- La respuesta correcta es re-emitir los certificados de los buzones movidos bajo
-- la autoridad del destino. Y eso NO hay que programarlo: `smime.Service` ya
-- emite de forma perezosa —si un buzón no tiene certificado, se lo emite al
-- enviar, con la autoridad de SU organización, que tras la fusión ya es la del
-- destino—. Así que basta con retirar el material que quedó sin anclaje y dejar
-- que el camino normal lo regenere. Cero criptografía aquí: esta función solo
-- decide QUÉ deja de valer; el material nuevo lo crea Mail con su KEK, que es la
-- única que puede sellar una clave privada.
--
-- QUÉ SE PIERDE, DICHO EXPLÍCITAMENTE. La clave privada anterior del buzón. No
-- es una política nueva: la renovación anual ya la sobrescribe (`SaveIdentity`
-- hace ON CONFLICT DO UPDATE), y hoy el servicio solo FIRMA —no descifra—, así
-- que ningún correo deja de poder leerse. La firma de los correos ya enviados
-- sigue siendo verificable: el certificado viaja dentro del propio mensaje.
--
-- SI EL DESTINO NO TIENE AUTORIDAD no se toca nada, y es importante que sea así:
-- la del origen se mueve tal cual con el UPDATE de siempre, los certificados
-- siguen encadenando y nadie tiene que redistribuir un ancla. Ese es también el
-- caso del RENOMBRADO de una organización (`id renameorg`), que usa este mismo
-- motor contra un destino que todavía no existe.

CREATE FUNCTION platform_merge_prepare(source text, target text) RETURNS text
LANGUAGE plpgsql
AS $$
DECLARE
    buzones integer;
BEGIN
    -- SOLO dentro de una fusión, y esto NO es decorativo: la RLS por sí sola no
    -- protege esta función. Una conexión normal de la aplicación lleva su
    -- `app.tenant` fijado y por tanto VE sus propias filas, así que sin esta
    -- guarda bastaba con llamar a platform_merge_prepare('mi-org','mi-org')
    -- para que una organización se borrase su propia autoridad y todos sus
    -- certificados. Un GRANT no lo evita: la fusión usa las MISMAS credenciales
    -- que el servicio (por diseño, ver el encabezado de internal/merge), así que
    -- lo único que distingue de verdad a una fusión es `app.merge`, que solo fija
    -- pg.InMergeTx.
    IF current_setting('app.merge', true) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION 'platform_merge_prepare: solo puede ejecutarse dentro de una transacción de fusión';
    END IF;
    -- Fusionar una organización consigo misma no es una fusión: es borrarle el
    -- S/MIME. El motor no lo hace, pero el coste de rechazarlo es una línea.
    IF source IS NULL OR target IS NULL OR source = target THEN
        RAISE EXCEPTION 'platform_merge_prepare: origen y destino deben ser distintos (% → %)', source, target;
    END IF;

    -- Sin autoridad en el destino no hay conflicto: mover es lo correcto.
    IF NOT EXISTS (SELECT 1 FROM smime_authorities WHERE tenant = target) THEN
        RETURN NULL;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM smime_authorities WHERE tenant = source) THEN
        RETURN NULL;
    END IF;

    DELETE FROM smime_identities WHERE tenant = source;
    GET DIAGNOSTICS buzones = ROW_COUNT;
    DELETE FROM smime_authorities WHERE tenant = source;

    RETURN format(
        'S/MIME: descartada la autoridad de %s (el destino ya tiene la suya); '
        '%s buzón(es) re-emitirán su certificado bajo la autoridad de %s al enviar su próximo correo',
        source, buzones, target);
END $$;

-- SECURITY INVOKER (el de por defecto, y aquí es obligatorio): bajo FORCE ROW
-- LEVEL SECURITY un SECURITY DEFINER tampoco se saltaría las políticas, y además
-- ejecutaría con los privilegios del dueño. La función ve las filas de las dos
-- organizaciones porque hereda el `app.merge='on'` de la transacción que la
-- llama; quien la protege de todo lo demás es la guarda de su primera línea.
COMMENT ON FUNCTION platform_merge_prepare(text, text) IS
    'Gancho de fusión de organizaciones: retira el material S/MIME que se queda sin anclaje. Lo llama services/id/internal/merge.';
