-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El logotipo propio de cada organización.
--
-- Hasta ahora la barra superior de las once superficies enseñaba nuestra marca a
-- todo el mundo. Una organización que trabaja todo el día dentro de la suite
-- quiere ver la suya, y quien no suba ninguna se queda con la nuestra teñida de un
-- color derivado de su slug, para que dos empresas distintas no se vean idénticas.
--
-- EL BINARIO VIVE AQUÍ, en `bytea`, y no en el almacén de objetos. Es una
-- desviación consciente de la norma de la casa: `platform/pkg/objectstore` existe y
-- es multi-tenant, pero `services/id` no lo conoce —no hay ni un import—, así que
-- usarlo significaría cablear S3 entero en el servicio que custodia el JWKS y la
-- KEK (bucket, secreto, Helm, Compose y cuatro entornos de OpenTofu) para mover
-- treinta kilobytes. El día que ID necesite guardar algo grande de verdad, ese es
-- el momento de cablearlo; un PNG de 112 px de alto no lo es.
--
-- Sin fila = sin logo propio = el de defecto. No se guarda el original que subió el
-- administrador: lo que se sirve es SIEMPRE lo que salió del reencuadre
-- (platform/pkg/logofit), re-codificado por nosotros, y guardar además el original
-- solo añadiría datos personales que custodiar sin nada que hacer con ellos.
--
-- Sin RLS, como el resto de tablas de este servicio: aquí la tenencia no se filtra
-- por política de fila, la resuelve el propio ID, que es quien la define.
CREATE TABLE org_branding (
    org_id     uuid PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    -- El PNG ya reencuadrado. El techo de 256 KiB es de sobra para 112 px de alto
    -- (un logo real ronda los 10-30 KB) y es la última barrera si alguna vez se
    -- toca el pipeline: la tabla no puede engordar por un fallo aguas arriba.
    logo_png   bytea NOT NULL CHECK (octet_length(logo_png) BETWEEN 1 AND 262144),
    -- Hash del contenido. Viaja en el token como parte de la URL (`?v=<hash>`) para
    -- que el logo se pueda servir como inmutable durante un año y aun así cambiar
    -- en cuanto el administrador suba otro: cambia el hash, cambia la URL.
    logo_hash  text NOT NULL CHECK (logo_hash ~ '^[a-f0-9]{16}$'),
    updated_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE org_branding IS
    'Logotipo propio de la organización, ya reencuadrado. Sin fila = marca de Sytue teñida por slug.';
