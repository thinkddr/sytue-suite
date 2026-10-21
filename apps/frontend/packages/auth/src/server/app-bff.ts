/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL BFF DE UNA APP DE LA SUITE, montado igual en todas.
 *
 * `createBff` es la pieza general; esto es la receta concreta que repetían siete
 * `src/lib/bff.ts` idénticos salvo el comentario de cabecera: config OIDC del
 * entorno, secreto de sesión del entorno, el prefijo de Astro como `basePath` y
 * cookies seguras salvo en dev. Los dos guards de sesión venían detrás, en otros
 * seis `src/lib/session.ts` que también eran el mismo fichero.
 *
 * Se devuelven JUNTOS —bff y guards— porque los guards necesitan ESE bff, y
 * pasarlo a mano en cada llamada es lo que empuja a volver a copiar el fichero.
 */
import type { APIContext } from 'astro';
import { returnToCanonico } from '@sytue/config';
import { createBff, type Bff, type BffContext } from './bff.js';
import { resolveOidcConfig, resolveSessionSecret, serverEnv, type EnvLike } from '../config.js';
import type { SytueUser } from '../types.js';

/** Lo que una página o un endpoint necesita de la sesión, y nada más. */
export interface ServerSession {
  user: SytueUser;
  /** SOLO para autorizar llamadas desde el SERVIDOR; jamás se envía al navegador. */
  accessToken: string;
}

/** La sesión de un endpoint API a partir de un BFF: null si no la hay (responde 401). */
export function sesionApi(bff: Bff): (ctx: APIContext) => Promise<ServerSession | null> {
  return async (ctx) => {
    const session = await bff.getSession(toBffContext(ctx));
    return session ? { user: session.user, accessToken: session.tokens.accessToken } : null;
  };
}

/** Adapta el contexto de Astro al que espera el BFF. */
export function toBffContext(ctx: APIContext): BffContext {
  return { request: ctx.request, cookies: ctx.cookies, url: ctx.url };
}

export interface BffDeApp {
  bff: Bff;
  /** Lee la sesión; si falta, lanza una Response de redirección al login. */
  requirePageSession: (ctx: APIContext) => Promise<ServerSession>;
  /** Variante para endpoints API: devuelve null (el endpoint responde 401). */
  getApiSession: (ctx: APIContext) => Promise<ServerSession | null>;
}

/**
 * Monta el BFF de una app y sus dos guards. Uso: `crearBffDeApp(import.meta.env)`.
 *
 * `basePath` sale de `buildEnv.BASE_URL` y NO de `serverEnv(...)` a propósito:
 * `serverEnv` deja que `process.env` gane sobre el entorno de build, que es justo
 * lo que hace falta para los secretos —congelados en el bundle SSR— pero sería un
 * desastre para el prefijo de Astro, que solo conoce el build. Un `BASE_URL`
 * suelto en el contenedor mandaría los logins de la app a otro sitio.
 */
export function crearBffDeApp(buildEnv: EnvLike): BffDeApp {
  const env = serverEnv(buildEnv);
  const isDev = env.DEV === 'true' || env.MODE === 'development';

  const bff = createBff({
    config: resolveOidcConfig(env),
    sessionSecret: resolveSessionSecret(env),
    // El prefijo bajo el que Astro sirve esta app. Sin él, el guard redirige
    // al login de la RAÍZ del host, que en producción es el 404 de Traefik.
    // Se omite la clave en vez de pasar `undefined`: con `exactOptionalPropertyTypes`
    // no son lo mismo, y el valor por defecto de `createBff` ya es el correcto.
    ...(buildEnv.BASE_URL === undefined ? {} : { basePath: buildEnv.BASE_URL }),
    secureCookies: !isDev,
  });

  return {
    bff,
    async requirePageSession(ctx) {
      const session = await bff.getSession(toBffContext(ctx));
      if (!session) {
        // La petición puede llegar REESCRITA por dentro (entrada org-ful con la
        // marca `?__org=`, ADR 0056): el returnTo sale en la forma canónica, no
        // en la interna, o tras el login la barra se quedaría con la marca.
        const returnTo = returnToCanonico(ctx.url, bff.basePath);
        throw ctx.redirect(
          `${bff.basePath}/auth/login?returnTo=${encodeURIComponent(returnTo)}`,
          302,
        );
      }
      return { user: session.user, accessToken: session.tokens.accessToken };
    },
    getApiSession: sesionApi(bff),
  };
}
