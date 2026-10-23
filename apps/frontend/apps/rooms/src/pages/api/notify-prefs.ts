/**
 * /api/notify-prefs — preferencias de notificación del usuario (Ola 13): palabras clave
 * y ventana «No molestar». Proxy BFF hacia el servicio Go Sytue Chat, autorizado por
 * cookie (el access token no toca el navegador).
 *
 *   GET  → mis preferencias { prefs }.
 *   POST → guarda { keywords, dndStart, dndEnd } (JSON, no form, contra el CSRF de Astro
 *          dev; el servicio expone PUT server-to-server, aquí se hace POST desde el
 *          navegador). Devuelve las prefs saneadas por el backend.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const prefs = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).getNotifyPrefs();
    return json({ prefs });
  } catch (err) {
    return json({ error: 'chat_error' }, 502);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { keywords?: unknown; dndStart?: unknown; dndEnd?: unknown };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  // Sanea la forma en el borde; el backend hace el saneo canónico (dedupe/recorte/rango).
  const keywords = Array.isArray(payload.keywords)
    ? payload.keywords.filter((k): k is string => typeof k === 'string')
    : [];
  const dndStart = Number.isFinite(payload.dndStart) ? Number(payload.dndStart) : 0;
  const dndEnd = Number.isFinite(payload.dndEnd) ? Number(payload.dndEnd) : 0;

  try {
    const prefs = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).putNotifyPrefs({ keywords, dndStart, dndEnd });
    return json({ prefs });
  } catch (err) {
    return json({ error: 'chat_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
