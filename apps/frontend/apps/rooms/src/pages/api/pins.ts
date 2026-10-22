/**
 * GET /api/pins?channelId= — mensajes fijados de un canal (proxy BFF hacia el
 * servicio Go Sytue Chat). Alimenta el panel «Fijados» y el contador de la
 * cabecera del canal.
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

  try {
    const pins = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).listPins(channelId);
    return json({ pins });
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
