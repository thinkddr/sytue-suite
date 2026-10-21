/**
 * Cliente del servicio Go Sytue Notepads para el BFF (SOLO servidor).
 *
 * El navegador nunca ve el access token: habla con `/api/*` de esta app usando la
 * cookie de sesión httpOnly, y es aquí donde se pone el Bearer. Así la
 * autorización la sigue aplicando el servicio recurso a recurso, sin que el
 * cliente pueda saltársela reenviando la petición a mano.
 */
import { NotesClient, bearer, HttpError } from '@sytue/api-clients';
import { serverEnv } from '@sytue/auth/server';
import type { APIContext } from 'astro';
import { getApiSession } from './session.js';

const env = serverEnv(import.meta.env);

/** URL interna de la API de Sytue Notepads (server-side). */
export function notesApiUrl(): string {
  // Por el Service INTERNO: esto corre en un pod y el nombre público no
  // resuelve desde dentro (`getaddrinfo ETIMEOUT`, medido el 2026-09-10). Sale
  // como 502 —«la llamada no llegó a ser HTTP»—. Mismo patrón que
  // `SYTUE_MAIL_INTERNAL_URL`; la pública queda de respaldo para un despliegue
  // que aún no tenga la variable nueva.
  return env.SYTUE_NOTES_INTERNAL_URL ?? env.SYTUE_NOTES_BASE_URL ?? 'https://notes.sytue.eu';
}

/** URL de la API de Sytue Calendar desde el BFF: `/api/suite` y la agenda del día. */
export function calendarApiUrl(): string {
  return (
    env.SYTUE_CALENDAR_API_URL ?? env.PUBLIC_SYTUE_CALENDAR_BASE_URL ?? 'https://calendar.sytue.eu'
  );
}

/** Respuesta JSON con `no-store`: son datos del usuario, no se cachean. */
export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/**
 * Resuelve la sesión y construye el cliente. Devuelve la respuesta 401 ya escrita
 * cuando no hay sesión, para que cada endpoint quepa en tres líneas.
 */
export async function conCliente(ctx: APIContext): Promise<{ client: NotesClient } | Response> {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  return {
    client: new NotesClient({ baseUrl: notesApiUrl(), auth: bearer(session.accessToken) }),
  };
}

/**
 * Traduce un fallo del servicio a la respuesta del BFF conservando su ESTADO.
 * Sin esto, un 404 de Notes llegaba al navegador como 500 y la interfaz no podía
 * distinguir «no existe» de «se ha roto algo».
 */
export function fallo(err: unknown): Response {
  if (err instanceof HttpError) {
    return json({ error: err.code ?? 'upstream_error' }, err.status);
  }
  return json({ error: 'upstream_error' }, 502);
}

/** Lee el cuerpo JSON de la petición; `{}` si viene vacío o mal formado. */
export async function cuerpo<T>(ctx: APIContext): Promise<T> {
  try {
    return (await ctx.request.json()) as T;
  } catch {
    return {} as T;
  }
}
