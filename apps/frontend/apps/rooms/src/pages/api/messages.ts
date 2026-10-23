/**
 * GET /api/messages?channelId=&before=&limit= — historico de mensajes de un canal
 * (proxy BFF hacia el servicio Go Sytue Chat). El tiempo real llega por WebSocket;
 * este endpoint sirve la carga inicial y la paginacion hacia atras.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const channelId = ctx.url.searchParams.get('channelId');
  if (!channelId) return json({ error: 'missing_channelId' }, 400);

  // Opciones sin claves `undefined` explicitas (exactOptionalPropertyTypes).
  const opts: { before?: string; limit?: number } = {};
  const before = ctx.url.searchParams.get('before');
  if (before) opts.before = before;
  const limitParamRaw = ctx.url.searchParams.get('limit');
  if (limitParamRaw) opts.limit = Math.min(Math.max(Number(limitParamRaw), 1), 100);

  try {
    const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
    const messages = await client.listMessages(channelId, opts);
    // Marcar el canal como leido al cargar (best-effort; no bloquea la respuesta).
    void client.markRead(channelId).catch(() => undefined);
    return json({ messages });
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
