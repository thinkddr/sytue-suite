/*
 * Wiring del BFF de @sytue/auth para Sytue DevX (SOLO servidor). Client OIDC
 * confidencial `sytue-cloud`. Emite la cookie de sesion httpOnly de la suite
 * (.sytue.eu). Importar unicamente desde endpoints/paginas SSR: nunca desde
 * islas de cliente (aqui vive el access token). docs/CONVENTIONS.md §4, §8.
 */
import {
  createBff,
  resolveOidcConfig,
  resolveSessionSecret,
  serverEnv,
  type BffContext,
} from '@sytue/auth/server';
import type { APIContext, AstroGlobal } from 'astro';

const env = serverEnv(import.meta.env);

export const bff = createBff({
  config: resolveOidcConfig(env),
  sessionSecret: resolveSessionSecret(env),
  // El prefijo bajo el que Astro sirve esta app. Sin él, el guard redirige
  // al login de la RAÍZ del host, que en producción es el 404 de Traefik.
  basePath: import.meta.env.BASE_URL,
  secureCookies: env.SYTUE_COOKIES_SECURE !== 'false',
});

export function toBffContext(ctx: APIContext | AstroGlobal): BffContext {
  return { request: ctx.request, cookies: ctx.cookies, url: new URL(ctx.request.url) };
}

/** URL base del control-plane del PaaS (Sytue DevX API, cloud.sytue.eu / dev :8090). */
export function cloudBaseUrl(): string {
  return env.SYTUE_CLOUD_API_URL ?? env.PUBLIC_SYTUE_CLOUD_API_URL ?? 'https://cloud.sytue.eu';
}
