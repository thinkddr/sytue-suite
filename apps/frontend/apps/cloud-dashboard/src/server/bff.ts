/**
 * BFF de autenticacion de Sytue DevX (client OIDC confidencial `sytue-cloud`).
 *
 * Singleton del lado servidor: implementa el flujo OIDC Authorization Code +
 * PKCE contra Sytue ID (discovery OIDC estandar) y emite la cookie de sesion
 * httpOnly de dominio .sytue.eu (sesion unica de la suite). El secreto del
 * client jamas llega al navegador. docs/CONVENTIONS.md §4, §8.
 */
import { createBff, resolveOidcConfig, resolveSessionSecret, serverEnv } from '@sytue/auth/server';
import type { APIContext } from 'astro';
import type { BffContext } from '@sytue/auth/server';

// serverEnv y no import.meta.env a secas: en el SSR COMPILADO el env de build
// queda congelado y las SYTUE_OIDC_* del contenedor solo viven en process.env
// (sin esto, el dashboard responde 500). Mismo patrón que portal y mail.
const env = serverEnv(import.meta.env);

let cached: ReturnType<typeof createBff> | null = null;

export function getBff(): ReturnType<typeof createBff> {
  if (cached) return cached;
  cached = createBff({
    config: resolveOidcConfig(env),
    sessionSecret: resolveSessionSecret(env),
    // El prefijo bajo el que Astro sirve esta app. Sin él, el guard redirige
    // al login de la RAÍZ del host, que en producción es el 404 de Traefik.
    // El prefijo bajo el que Astro sirve esta app. `SYTUE_BASE_PATH` lo ANULA
    // porque una misma imagen puede servir dos superficies: Sytue Admin cuelga
    // de `/admin` y la consola de plataforma se sirve en la RAÍZ de su host. Con
    // el valor del build a secas, el guard de platform.sytue.eu redirigía a
    // `/admin/auth/login`, que ahí no existe: 404 al intentar entrar.
    basePath: env.SYTUE_BASE_PATH ?? import.meta.env.BASE_URL,
    secureCookies: env.SYTUE_COOKIES_SECURE !== 'false',
  });
  return cached;
}

/** Adapta un APIContext de Astro al BffContext estandar (Request/cookies/URL). */
export function toBffContext(ctx: APIContext): BffContext {
  return { request: ctx.request, cookies: ctx.cookies, url: ctx.url };
}
