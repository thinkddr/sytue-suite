/*
 * Wiring del BFF de @sytue/auth para Sytue Admin (SOLO servidor). Client OIDC
 * confidencial `sytue-console`. Emite la cookie de sesión httpOnly de la suite
 * (.sytue.eu). La administración de identidad exige rol admin/owner (guard).
 * Importar únicamente desde endpoints/páginas SSR. docs/CONVENTIONS.md §4, §8.
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

/** URL base de la API de administración de Sytue ID (identidad/SCIM) vía control-plane. */
/**
 * Sytue ID, PARA HABLAR DESDE DENTRO DEL CLÚSTER.
 *
 * Iba por el nombre público y desde un pod eso no resuelve: `getaddrinfo
 * ETIMEOUT id.sytue.eu`, medido el 2026-09-10 desde el propio contenedor de la
 * consola. Salía como 502 en `/admin/api/admin/invitations` y en
 * `/admin/api/admin/users/bulk` —ahí un 502 significa «la llamada no llegó a ser
 * HTTP», porque un 401 o un 403 se propagan con su código—. Por el Service
 * interno, la misma ruta contesta 401.
 *
 * Es exactamente lo que ya se arregló para Mail y Drive el 2026-08-14, con el
 * mismo síntoma y en el mismo tipo de sitio.
 *
 * `SYTUE_ADMIN_API_URL` y el issuer se conservan de respaldo para no romper un
 * despliegue que aún no tenga la variable nueva. El ISSUER NO SE TOCA: es lo que
 * se compara con el `iss` del token y tiene que seguir siendo el público.
 */
export function adminBaseUrl(): string {
  return (
    env.SYTUE_ID_INTERNAL_URL ??
    env.SYTUE_ADMIN_API_URL ??
    env.SYTUE_OIDC_ISSUER ??
    'https://id.sytue.eu'
  );
}

/**
 * Sytue Hub PARA EL NAVEGADOR: la búsqueda global de la shell y el `preconnect`
 * del layout. Tiene que ser la pública, porque quien la abre es el navegador.
 */
export function hubBaseUrl(): string {
  return env.SYTUE_HUB_URL ?? 'https://hub.sytue.eu';
}

/**
 * Sytue Hub PARA HABLAR DESDE DENTRO. Misma historia que `adminBaseUrl`:
 * `hub.sytue.eu` no resuelve desde un pod (comprobado, `ETIMEOUT`) y
 * `http://sytue-one-hub` contesta.
 *
 * Van SEPARADAS y no es duplicar: la de arriba viaja al navegador como prop, así
 * que apuntarla al Service del clúster dejaría la búsqueda global sin servidor
 * al que llamar. Son dos destinos distintos para el mismo servicio.
 */
export function hubInternalUrl(): string {
  return env.SYTUE_HUB_INTERNAL_URL ?? env.SYTUE_HUB_URL ?? 'https://hub.sytue.eu';
}

/** URL base del Sytue Integration Gateway (catálogo + instancias de integraciones). */
export function gatewayBaseUrl(): string {
  return env.SYTUE_INTEGRATIONS_BASE_URL ?? 'https://integrations.sytue.eu';
}

/*
 * Mail y Drive, para las llamadas de SERVIDOR del panel de cumplimiento
 * (retenciones legales y eDiscovery).
 *
 * PRIMERO EL NOMBRE INTERNO. Estas llamadas salen del BFF, que corre dentro del
 * clúster, así que van al Service y no al nombre público. Con el público hay que
 * salir al ingress y volver a entrar, y eso en producción no funciona: medido el
 * 2026-08-14, las dos ramas del panel devolvían 502 —que en `attempt()` significa
 * «no hubo respuesta HTTP», porque un 401 o un 403 se propagan con su código—.
 * Las rutas existen y los servicios contestan desde fuera; lo que fallaba era el
 * viaje de vuelta.
 *
 * Es el mismo patrón que `SYTUE_INTEGRATIONS_INTERNAL_URL` en la superficie de
 * plataforma. Se conserva `*_API_URL` como respaldo para no romper un despliegue
 * que aún no tenga la variable nueva: si no está, se comporta como antes.
 */
export function mailBaseUrl(): string {
  return env.SYTUE_MAIL_INTERNAL_URL ?? env.SYTUE_MAIL_API_URL ?? 'https://mail.sytue.eu';
}

export function driveBaseUrl(): string {
  return env.SYTUE_DRIVE_INTERNAL_URL ?? env.SYTUE_DRIVE_API_URL ?? 'https://drive.sytue.eu';
}

export function notesBaseUrl(): string {
  return env.SYTUE_NOTES_INTERNAL_URL ?? env.SYTUE_NOTES_API_URL ?? 'https://notes.sytue.eu';
}

/**
 * Rooms, para la administración de canales de la organización. Mismo orden y por
 * el mismo motivo que Mail y Drive: la llamada sale del BFF, que corre dentro del
 * clúster, y salir al ingress para volver a entrar da 502.
 */
export function roomsBaseUrl(): string {
  return env.SYTUE_ROOMS_INTERNAL_URL ?? env.SYTUE_ROOMS_API_URL ?? 'https://rooms.sytue.eu';
}

/**
 * Sytue Cast, para la cola de denuncias del operador (consola de plataforma). Por
 * el Service interno, como Rooms; el respaldo es el nombre del compose.
 */
export function castBaseUrl(): string {
  return env.SYTUE_CAST_API_URL ?? 'http://cast:8099';
}

/**
 * Sytue Docs, para la política de peticiones de firma de fuera (ADR 0059). Por el
 * Service interno, como Rooms.
 */
export function docsBaseUrl(): string {
  return env.SYTUE_DOCS_INTERNAL_URL ?? env.PUBLIC_SYTUE_DOCS_BASE_URL ?? 'https://docs.sytue.eu';
}

/** Sytue ERP, para los permisos por módulo. Por el Service interno, como Rooms. */
export function erpBaseUrl(): string {
  return env.SYTUE_SYERP_INTERNAL_URL ?? env.SYTUE_SYERP_BASE_URL ?? 'https://erp.sytue.eu';
}
