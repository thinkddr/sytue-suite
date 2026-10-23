/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Wiring del BFF de @sytue/auth para Sytue Drive (SOLO servidor). Client OIDC
 * confidencial `sytue-drive`. Emite la cookie de sesion httpOnly de la suite
 * (.sytue.eu). Importar unicamente desde endpoints/paginas SSR.
 */
import {
  createBff,
  resolveOidcConfig,
  resolveSessionSecret,
  serverEnv,
  resolveIntegrationsUrl,
  type BffContext,
} from '@sytue/auth/server';
import { DriveClient, bearer } from '@sytue/api-clients';
import type { APIContext, AstroGlobal } from 'astro';

const env = serverEnv(import.meta.env);

export const bff = createBff({
  config: resolveOidcConfig(env),
  sessionSecret: resolveSessionSecret(env),
  // El prefijo bajo el que Astro sirve esta app. Sin él, el guard redirige
  // al login de la RAÍZ del host, que en producción es el 404 de Traefik.
  basePath: import.meta.env.BASE_URL,
  secureCookies: env.SYTUE_SECURE_COOKIES !== 'false',
});

export function toBffContext(ctx: APIContext | AstroGlobal): BffContext {
  return { request: ctx.request, cookies: ctx.cookies, url: new URL(ctx.request.url) };
}

/**
 * Resuelve la sesion de la cookie httpOnly y construye un DriveClient autenticado
 * con el access token de Sytue ID. SOLO servidor: el token nunca abandona el BFF.
 * Devuelve `null` si no hay sesion valida (el endpoint debe responder 401).
 */
export async function driveClientFor(ctx: APIContext): Promise<DriveClient | null> {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return null;
  return new DriveClient({
    baseUrl: driveBaseUrl(),
    auth: bearer(session.tokens.accessToken),
  });
}

/**
 * Petición autenticada al servicio Go de Drive con el access token de la sesión
 * httpOnly. Para endpoints que el DriveClient de @sytue/api-clients aún no cubre
 * (recientes, favoritos, cuota). SOLO servidor: el token nunca abandona el BFF.
 * Devuelve `null` si no hay sesión válida (el endpoint debe responder 401).
 */
export async function driveFetch(
  ctx: APIContext,
  path: string,
  init?: RequestInit,
): Promise<Response | null> {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return null;
  const headers = new Headers(init?.headers);
  headers.set('authorization', `Bearer ${session.tokens.accessToken}`);
  try {
    return await fetch(`${driveBaseUrl()}${path}`, { ...init, headers });
  } catch {
    // UN FALLO DE RED NO ES UN FALLO DE LA RUTA. Sin esto, `fetch` LANZA —DNS
    // que no resuelve, conexión rechazada, timeout— y la excepción sube hasta
    // Astro, que responde 500. Y un 500 aquí miente dos veces: dice que el fallo
    // es del BFF cuando está en el camino hacia el servicio, y no se parece al
    // 502 que estas mismas rutas devuelven cuando el servicio contesta mal.
    //
    // Pasó el 2026-09-10: `favorites` y `quota` daban 500 y `files` daba 502 por
    // el MISMO `getaddrinfo ETIMEOUT` —la única diferencia era que `files` tiene
    // `try/catch`—. Dos códigos para un fallo es lo que hizo que pareciera dos
    // problemas distintos.
    //
    // Se devuelve una respuesta 502 en vez de propagar: las veintidós rutas que
    // llaman aquí ya saben tratar un `!res.ok`, así que arreglarlo en el sitio
    // compartido las cubre todas sin tocar ninguna.
    return new Response(
      JSON.stringify({ error: 'drive_unreachable', detail: 'Drive service unavailable' }),
      {
        status: 502,
        headers: { 'content-type': 'application/json; charset=utf-8' },
      },
    );
  }
}

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

/**
 * URL del servicio Go de Sytue Drive, PARA LLAMARLO DESDE AQUÍ.
 *
 * El Service del clúster primero. Sus tres usos —`driveClientFor`, `driveFetch`
 * y la resolución de `/files/[id]`— son de servidor a servidor: ninguna de esas
 * URL llega al navegador, así que aquí el nombre público solo servía para salir
 * al ingress y volver a entrar, que desde un pod no funciona.
 *
 * Medido en el piloto el 2026-09-10, con la variable ya presente en el
 * contenedor y esta función sin mirarla:
 *
 *   TypeError: getaddrinfo ETIMEOUT drive.sytue.eu
 *     at .../chunks/favorites_*.mjs
 *     at .../chunks/quota_*.mjs
 *
 * El fetch ni llega a ser HTTP, así que la excepción sube sin capturar y la ruta
 * devuelve 500 —no el 502 que este BFF usa para «el servicio contestó mal»—.
 * `files` sí lo traduce a 502 porque tiene `try/catch`; el mismo fallo con dos
 * caras distintas, que es lo que costó verlo.
 */
export function driveBaseUrl(): string {
  return (
    env.SYTUE_DRIVE_INTERNAL_URL ?? env.PUBLIC_SYTUE_DRIVE_BASE_URL ?? 'https://drive.sytue.eu'
  );
}

/**
 * URL del servicio Go de Sytue Docs. Drive lo necesita para las operaciones de
 * página sobre un PDF (Sytue PDF): el motor vive en Docs y el fichero en Drive,
 * así que el BFF de Drive hace de puente y el token no sale de aquí.
 */
export function docsBaseUrl(): string {
  // El Service del clúster primero, por lo mismo que `driveBaseUrl`: esta URL no
  // llega al navegador, la usa el puente de PDF desde este BFF.
  return env.SYTUE_DOCS_INTERNAL_URL ?? env.PUBLIC_SYTUE_DOCS_BASE_URL ?? 'https://docs.sytue.eu';
}

/** Server-to-server gateway calls for the external-storage consent flow. */
export function integrationsBaseUrl(): string {
  return resolveIntegrationsUrl(env);
}
