/**
 * Perfil propio.
 *
 *   GET   /api/me   Estado de la cuenta (los siete campos, `mfa_enabled`…).
 *   PATCH /api/me   Guarda los campos que vengan; los ausentes se conservan.
 *
 * Va por el BFF y no directo desde el navegador porque el access token NO sale
 * al cliente: la sesión vive en una cookie de este origen y es aquí donde se
 * canjea por el token que entiende el servicio.
 */
import type { APIRoute } from 'astro';
import { LOCALE_COOKIE } from '@sytue/i18n';
import { proxyId } from '../../lib/bff.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => proxyId(ctx, '/api/v1/me');

/** Los campos que esta pantalla edita. El resto del perfil no se toca desde aquí. */
const CAMPOS = [
  'name',
  'given_name',
  'family_name',
  'title',
  'department',
  'location',
  'locale',
] as const;

/**
 * La preferencia de idioma vale en TODA la suite, no solo aquí.
 *
 * La cookie se fija con el dominio de segundo nivel —el mismo criterio que el
 * BFF para la de sesión—, que es lo que hace que `mail.<dominio>` y las demás la
 * lean. Sin esto, elegir el idioma en «Mi cuenta» solo cambiaba «Mi cuenta» y
 * había que volver a elegirlo en cada superficie. Es lo que ya hacía el
 * formulario de Go (`setLocaleCookie`), y se replica para no perderlo al migrar.
 */
function fijarIdioma(ctx: Parameters<APIRoute>[0], locale: string): void {
  const labels = ctx.url.hostname.split('.');
  if (labels.length < 2) return;
  ctx.cookies.set(LOCALE_COOKIE, locale, {
    path: '/',
    domain: `.${labels.slice(-2).join('.')}`,
    maxAge: 365 * 24 * 60 * 60,
    sameSite: 'lax',
    httpOnly: false, // preferencia de idioma, no credencial: la suite la lee
    secure: ctx.url.protocol === 'https:',
  });
}

export const PATCH: APIRoute = async (ctx) => {
  let body: Record<string, unknown>;
  try {
    body = (await ctx.request.json()) as Record<string, unknown>;
  } catch {
    return new Response(null, { status: 400 });
  }

  // Se reenvía SOLO la lista blanca: dejar pasar el cuerpo entero permitiría
  // intentar escribir campos que aquí no se enseñan ni se validan. Y solo lo que
  // VIENE, para que el PATCH del servicio conserve lo ausente.
  const parche: Record<string, string> = {};
  for (const campo of CAMPOS) {
    const v = body[campo];
    if (typeof v === 'string') parche[campo] = v.trim();
  }
  if (Object.keys(parche).length === 0) return new Response(null, { status: 400 });
  if (parche.name !== undefined && parche.name === '') return new Response(null, { status: 400 });

  const res = await proxyId(ctx, '/api/v1/me', {
    method: 'PATCH',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(parche),
  });
  if (res.ok && parche.locale) fijarIdioma(ctx, parche.locale);
  return res;
};
