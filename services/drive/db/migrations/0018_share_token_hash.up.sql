-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El secreto del enlace público deja de guardarse en claro.
--
-- `shares.token` ES la credencial: quien lo tiene abre el fichero sin iniciar
-- sesión. Guardado en claro, cualquiera que lea la base —una copia de seguridad
-- extraviada, un volcado de soporte, un operador con acceso al motor— se lleva
-- todos los enlaces vivos y su contenido. Un hash lo impide y no cuesta nada
-- funcionalmente: al validar solo hay que COMPARAR, nunca recuperar el original.
--
-- SHA-256 a secas y no Argon2id: el token lo genera el servicio con 32 bytes de
-- entropía, así que no hay diccionario que probar. Argon2 aquí solo añadiría
-- coste a cada apertura de enlace (el camino más caliente del servicio) sin
-- ganar nada — es la misma razón por la que `mfa_continuations` guarda
-- `token_hash` y no un hash de contraseña.
--
-- No se pierde ninguna función: la interfaz ya emitía la URL SOLO al crear el
-- enlace y los listados nunca la devolvían (ShareDialog en FileBrowser.tsx). Los
-- enlaces ya repartidos siguen funcionando: abajo se hashean los existentes.
--
-- ⚠️ ORDEN DE DESPLIEGUE: esta migración ELIMINA `token`, así que el binario
-- anterior deja de resolver enlaces en cuanto se aplica. Aplicar con el servicio
-- ya actualizado (o en la misma ventana), no antes.

ALTER TABLE shares ADD COLUMN IF NOT EXISTS token_hash text;

-- Los enlaces ya repartidos siguen valiendo: se guarda el hash del token que ya
-- tiene el destinatario. `sha256()` es nativo desde PostgreSQL 11, sin pgcrypto.
UPDATE shares
   SET token_hash = encode(sha256(token::bytea), 'hex')
 WHERE token IS NOT NULL
   AND token_hash IS NULL;

-- Un token vivo resuelve a UN enlace. El índice es único y parcial (kind='link'
-- es minoría) y además es el que sirve la búsqueda del resolutor.
CREATE UNIQUE INDEX IF NOT EXISTS shares_token_hash_uniq
    ON shares (token_hash) WHERE token_hash IS NOT NULL;

-- La columna `token` NO se borra aquí, y no es un olvido: esta migración es el
-- «expand» y el borrado es el «contract», que va en un despliegue posterior.
--
-- Durante un rollout de Helm conviven las dos versiones del servicio, y la
-- ANTERIOR todavía lee `shares.token`. Borrar la columna en este mismo paso la
-- hace fallar mientras dure el relevo — y, peor, deja el rollback sin salida:
-- `helm rollback` devuelve el chart, no la columna, así que la vuelta atrás
-- llevaría a una versión que consulta algo que ya no existe. El gate
-- `platform/pkg/migcheck` lo bloquea por eso.
--
-- El objetivo de seguridad NO se pospone del todo: desde esta migración el
-- servicio resuelve por `token_hash` y ya no escribe tokens en claro, así que
-- los enlaces NUEVOS no engordan la columna. Lo que sobrevive un despliegue más
-- son los tokens ya existentes.
--
-- SIGUIENTE PASO, cuando la versión anterior ya no esté viva:
--     ALTER TABLE shares DROP CONSTRAINT IF EXISTS shares_link_token;
--     ALTER TABLE shares DROP COLUMN IF EXISTS token;
-- (el CHECK `shares_link_token` de 0001 exige token NOT NULL en los enlaces, así
-- que hay que quitarlo antes o en el mismo paso).

-- El resolutor recibe ahora el HASH, no el token: así el secreto no viaja al
-- motor y no acaba en `pg_stat_statements` ni en el log de consultas lentas.
-- Cambiar el parámetro obliga a recrear la función (mismo motivo que en 0003).
DROP FUNCTION IF EXISTS drive_resolve_share_tenant(text);
CREATE FUNCTION drive_resolve_share_tenant(p_token_hash text)
RETURNS SETOF text
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
BEGIN
    -- Salto de RLS acotado: activo SOLO durante la lectura de `shares` de abajo
    -- (ver 0003_fix_link_resolver para por qué SECURITY DEFINER no basta).
    PERFORM set_config('app.resolve_link', 'on', true);
    RETURN QUERY
        SELECT tenant
        FROM shares
        WHERE token_hash = p_token_hash
          AND revoked_at IS NULL
          AND (expires_at IS NULL OR expires_at > now())
        LIMIT 1;
    -- Restaura el aislamiento normal para el resto de la transacción.
    PERFORM set_config('app.resolve_link', 'off', true);
END;
$$;

REVOKE ALL ON FUNCTION drive_resolve_share_tenant(text) FROM PUBLIC;
