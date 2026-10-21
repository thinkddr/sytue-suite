/**
 * Wiring del BFF de @sytue/auth para el portal (SOLO servidor).
 *
 * Resuelve la configuración OIDC del client confidencial `sytue-one` desde el
 * entorno (nunca literales) y crea el BFF (login/callback/logout/session/
 * refresh) que emite una cookie de sesión host-only `__Host-`, httpOnly.
 * Importar únicamente desde endpoints/páginas SSR; jamás desde islas cliente.
 */
import {
  createBff,
  resolveOidcConfig,
  resolveSessionSecret,
  serverEnv,
  type BffContext,
} from '@sytue/auth/server';
import type { APIContext, AstroGlobal } from 'astro';
import type { SytueUser } from '@sytue/auth';

const env = serverEnv(import.meta.env);

/** BFF singleton del portal (client OIDC `sytue-one`). */
export const bff = createBff({
  config: resolveOidcConfig(env),
  sessionSecret: resolveSessionSecret(env),
  // El prefijo bajo el que Astro sirve esta app. Sin él, el guard redirige
  // al login de la RAÍZ del host, que en producción es el 404 de Traefik.
  basePath: import.meta.env.BASE_URL,
  // En dev local (http://*.sytue.localhost) las cookies no son Secure.
  secureCookies: env.SYTUE_SECURE_COOKIES !== 'false',
});

/** Adapta Astro (APIContext o Astro global) al BffContext mínimo. */
export function toBffContext(ctx: APIContext | AstroGlobal): BffContext {
  return { request: ctx.request, cookies: ctx.cookies, url: new URL(ctx.request.url) };
}

/** URL base del Hub para la shell (cliente). */
export function hubBaseUrl(): string {
  return env.PUBLIC_SYTUE_HUB_BASE_URL ?? env.SYTUE_HUB_BASE_URL ?? 'https://hub.sytue.eu';
}

/**
 * Sytue Hub PARA HABLAR DESDE DENTRO DEL CLÚSTER.
 *
 * `hubBaseUrl()` es la pública y tiene que seguir siéndolo: viaja al navegador
 * como prop. Pero un BFF que llama al Hub corre en un pod, y ahí el nombre
 * público NO RESUELVE: `getaddrinfo ETIMEOUT hub.sytue.eu`, medido el
 * 2026-09-10 desde el propio contenedor. Sale como 502 —«la llamada no llegó a
 * ser HTTP»—, mientras que por el Service interno la misma ruta contesta.
 *
 * Es el patrón que ya usan `SYTUE_MAIL_INTERNAL_URL` y `SYTUE_DRIVE_INTERNAL_URL`
 * desde que el panel de cumplimiento tuvo este mismo fallo el 2026-08-14. Con
 * respaldo a la pública para no romper un despliegue sin la variable nueva.
 */
export function hubInternalUrl(): string {
  return env.SYTUE_HUB_INTERNAL_URL ?? env.SYTUE_HUB_BASE_URL ?? hubBaseUrl();
}

/** Sytue Drive desde el BFF: disponibilidad del almacenamiento externo. */
export function driveInternalUrl(): string {
  return (
    env.SYTUE_DRIVE_INTERNAL_URL ??
    env.SYTUE_DRIVE_BASE_URL ??
    env.PUBLIC_SYTUE_DRIVE_BASE_URL ??
    'https://drive.sytue.eu'
  );
}

/** URL base del Sytue Integration Gateway (apps conectadas de la org). Solo servidor. */
export function gatewayBaseUrl(): string {
  return env.SYTUE_INTEGRATIONS_BASE_URL ?? 'https://integrations.sytue.eu';
}

/**
 * URL pública de Sytue ID (el issuer OIDC). Es también el origen del perfil de
 * Apple (/account/apple.mobileconfig) y de las coordenadas de los clientes
 * nativos (/.well-known/sytue-clients.json).
 */
/**
 * Sytue ID. SOLO SE USA DESDE EL SERVIDOR —el onboarding y los dispositivos, en
 * `.astro` y en el BFF—, así que va por el Service interno: desde un pod el
 * nombre público no resuelve (`ETIMEOUT`, medido el 2026-09-10) y la llamada
 * muere antes de ser HTTP.
 *
 * EL ISSUER SIGUE SIENDO EL RESPALDO Y NO SE TOCA en ningún otro sitio: es lo
 * que se compara con el `iss` del token y ahí tiene que ser el público.
 */
export function idBaseUrl(): string {
  return (env.SYTUE_ID_INTERNAL_URL ?? env.SYTUE_OIDC_ISSUER ?? 'https://id.sytue.eu').replace(
    /\/$/,
    '',
  );
}

/** Sesión resuelta del lado servidor: usuario + access token (uso interno). */
export interface ServerSession {
  user: SytueUser;
  accessToken: string;
}

/**
 * Lee la sesión para endpoints API (BFF): devuelve null si no hay sesión válida
 * (el endpoint responde 401). El access token solo se usa en el servidor para
 * autorizar llamadas federadas; jamás se envía al navegador.
 */
export async function getApiSession(ctx: APIContext): Promise<ServerSession | null> {
  const session = await bff.getSession(toBffContext(ctx));
  return session ? { user: session.user, accessToken: session.tokens.accessToken } : null;
}
