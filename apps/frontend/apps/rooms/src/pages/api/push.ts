/**
 * /api/push — Web Push (Ola 18). Proxy BFF hacia el servicio Go Sytue Chat, autorizado por
 * cookie (el access token no toca el navegador).
 *
 *   GET  → { key }  clave pública VAPID (applicationServerKey); vacía = push deshabilitado.
 *   POST → { action: 'subscribe', endpoint, keys } registra la suscripción del navegador.
 *          { action: 'unsubscribe', endpoint }     la da de baja.
 *   Se usa POST (JSON, no form) contra el CSRF de Astro dev; el servicio expone las
 *   mutaciones bajo /me/push/* sin CSRF de navegador.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const key = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).getPushKey();
    return json({ key });
  } catch {
    return json({ error: 'chat_error' }, 502);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: {
    action?: unknown;
    endpoint?: unknown;
    keys?: { p256dh?: unknown; auth?: unknown };
  };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
  const endpoint = typeof payload.endpoint === 'string' ? payload.endpoint : '';
  if (!endpoint) return json({ error: 'invalid_endpoint' }, 400);

  try {
    if (payload.action === 'unsubscribe') {
      await client.unsubscribePush(endpoint);
      return json({ ok: true });
    }
    const p256dh = typeof payload.keys?.p256dh === 'string' ? payload.keys.p256dh : '';
    const auth = typeof payload.keys?.auth === 'string' ? payload.keys.auth : '';
    if (!p256dh || !auth) return json({ error: 'invalid_keys' }, 400);
    await client.subscribePush({ endpoint, keys: { p256dh, auth } });
    return json({ ok: true });
  } catch {
    return json({ error: 'chat_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
