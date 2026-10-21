/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Wiring del BFF de @sytue/auth para Sytue Docs (SOLO servidor). Client OIDC
 * confidencial `sytue-docs`. Emite la cookie de sesion httpOnly de la suite
 * (.sytue.eu). Importar unicamente desde endpoints/paginas SSR.
 */
import {
  createBff,
  resolveOidcConfig,
  resolveSessionSecret,
  serverEnv,
  type BffContext,
} from '@sytue/auth/server';
import type { APIContext, AstroGlobal } from 'astro';

// El env EFECTIVO del servidor (import.meta.env de build + process.env de runtime):
// en el SSR compilado import.meta.env está congelado, process.env es la fuente viva.
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
 * Sytue Hub PARA EL NAVEGADOR. Su único llamante es `AppLayout.astro`, que la
 * pasa como prop a la shell: de ahí salen las notificaciones y su SSE.
 *
 * NO LLEVA LA URL INTERNA, y la nota es para que no vuelva a metérsele. Se hizo
 * el 2026-09-10 por parecerse a los helpers de otras apps que sí consume el BFF,
 * y el resultado fue que el navegador recibía
 * `http://sytue-one-hub.sytue.svc.cluster.local/...`: un nombre que no existe
 * fuera del clúster y además en claro, así que la página lo bloqueaba por
 * contenido inseguro y las notificaciones desaparecían de todas las superficies
 * de Docs.
 *
 * Si algún día el BFF de Docs necesita hablar con el Hub, va una función APARTE
 * —`hubInternalUrl()`, como en console, drive y portal—, no un `??` aquí.
 */
export function hubBaseUrl(): string {
  return env.PUBLIC_SYTUE_HUB_BASE_URL ?? env.SYTUE_HUB_BASE_URL ?? 'https://hub.sytue.eu';
}

/** URL del servicio Go de Sytue Docs (docs.sytue.eu, dev :8083). */
export function docsBaseUrl(): string {
  return env.PUBLIC_SYTUE_DOCS_BASE_URL ?? 'https://docs.sytue.eu';
}

/** URL del servicio Go de Sytue Drive (drive.sytue.eu, dev :8082): de ahí salen los
 *  ficheros que se abren en el editor con «Abrir con Sytue Docs». */
export function driveBaseUrl(): string {
  // Igual que el Hub: el BFF de Docs pide ficheros a Drive desde dentro.
  return (
    env.SYTUE_DRIVE_INTERNAL_URL ??
    env.PUBLIC_SYTUE_DRIVE_BASE_URL ??
    env.SYTUE_DRIVE_BASE_URL ??
    'https://drive.sytue.eu'
  );
}

/** URL del servicio Go de Sytue Cast, SOLO para el servidor. `SYTUE_CAST_API_URL` es la
 *  interna del clúster (el chart de la suite la pone en todos los frontales); el BFF de
 *  «Presentar con Cast» le reenvía la posición de la diapositiva con el token de quien
 *  presenta. */
export function castBaseUrl(): string {
  return (
    env.SYTUE_CAST_API_URL ??
    env.PUBLIC_SYTUE_CAST_BASE_URL ??
    'https://cast.sytue.eu'
  ).replace(/\/+$/, '');
}

/** URL del servicio Go de Sytue Rooms, SOLO para el servidor: de ahí salen la sala de un
 *  documento y quién está en la llamada. Por el Service interno (el nombre público no
 *  resuelve desde dentro del clúster). */
export function roomsBaseUrl(): string {
  return (
    env.SYTUE_ROOMS_INTERNAL_URL ??
    env.SYTUE_ROOMS_API_URL ??
    env.PUBLIC_SYTUE_ROOMS_BASE_URL ??
    'https://rooms.sytue.eu'
  ).replace(/\/+$/, '');
}

/** URL pública de la app de Sytue Rooms: adonde se abre la ventana de la llamada. */
export function roomsPublicUrl(): string {
  return (env.PUBLIC_SYTUE_ROOMS_BASE_URL ?? 'https://rooms.sytue.eu').replace(/\/+$/, '');
}

/** URL del Sytue Integration Gateway (mint de embed L4). SOLO servidor: destino
 *  interno de confianza al que el BFF reenvía el token del usuario (passthrough). */
export function integrationsBaseUrl(): string {
  return env.SYTUE_INTEGRATIONS_BASE_URL ?? 'https://integrations.sytue.eu';
}
