/**
 * BFF de Sytue Admin (client OIDC confidencial `sytue-console`). Singleton de
 * servidor: OIDC Authorization Code + PKCE contra Sytue ID (id.sytue.eu) y cookie
 * de sesión host-only `__Host-`, httpOnly. Las vistas de admin exigen rol
 * admin/owner (guard). docs/CONVENTIONS.md §4, §8.
 */
import { createBff, resolveOidcConfig, resolveSessionSecret, serverEnv } from '@sytue/auth/server';
import type { APIContext } from 'astro';
import type { BffContext } from '@sytue/auth/server';

// serverEnv y no import.meta.env a secas: en el SSR COMPILADO el env de build
// queda congelado y las SYTUE_OIDC_* del contenedor solo viven en process.env
// (sin esto, toda la consola responde 500). Mismo patrón que portal y mail.
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

export function toBffContext(ctx: APIContext): BffContext {
  return { request: ctx.request, cookies: ctx.cookies, url: ctx.url };
}

/**
 * Esta imagen sirve DOS superficies distintas con dos client OIDC distintos:
 * `sytue-console` en admin.<dominio>, que administra UNA organización, y
 * `sytue-platform` en platform.<dominio>, que administra la plataforma entera.
 *
 * Se deriva del client_id y no de una variable nueva porque el client_id YA es la
 * identidad de la superficie —lo fija el chart por despliegue y sin él no se puede
 * ni iniciar sesión—, así que no puede quedarse a medio configurar como se queda
 * una bandera que alguien tiene que acordarse de poner.
 *
 * Solo decide qué se OFRECE. Quién entra de verdad lo sigue decidiendo Sytue ID en
 * cada petición (allowlist `platform_admins` + segundo factor + elevación viva):
 * esto no es una comprobación de seguridad y no debe usarse como tal.
 */
export function isPlatformSurface(): boolean {
  return env.SYTUE_OIDC_CLIENT_ID === 'sytue-platform';
}
