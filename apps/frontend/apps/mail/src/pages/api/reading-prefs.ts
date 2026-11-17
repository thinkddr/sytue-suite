/**
 * /api/reading-prefs — preferencias de LECTURA de la cuenta (a dónde se va tras
 * archivar y qué hace deslizar una fila en el móvil), proxy BFF al servicio Go
 * (REST /api/v1/reading-prefs). Viven en la cuenta, no en el navegador.
 *
 * GET lee (en camelCase); POST guarda (el checkOrigin CSRF de Astro rechaza el PUT
 * del navegador, así que el BFF traduce POST → PUT, como /api/undo-send). Qué
 * valores valen lo decide el servicio: su 400 se propaga para que el cliente no dé
 * por guardado algo que no se guardó.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, mailPut } from '../../lib/mailrest.js';

export const prerender = false;

interface RawPrefs {
  auto_advance?: string;
  swipe_right?: string;
  swipe_left?: string;
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await mailGet(session.accessToken, '/api/v1/reading-prefs');
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as RawPrefs;
    return json({
      autoAdvance: body.auto_advance ?? '',
      swipeRight: body.swipe_right ?? '',
      swipeLeft: body.swipe_left ?? '',
    });
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as Record<string, unknown>;
  const texto = (v: unknown): string => (typeof v === 'string' ? v : '');
  try {
    const res = await mailPut(session.accessToken, '/api/v1/reading-prefs', {
      auto_advance: texto(body['autoAdvance']),
      swipe_right: texto(body['swipeRight']),
      swipe_left: texto(body['swipeLeft']),
    });
    if (!res.ok)
      return json({ error: 'mail_error', status: res.status }, res.status === 400 ? 400 : 502);
    return json({ ok: true });
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
