/**
 * GET /api/around?channelId=&message=&limit= — ventana de mensajes CENTRADA en un
 * mensaje objetivo (permalink / scroll-to-message, Ola 16), proxy BFF hacia el
 * servicio Go Sytue Chat. A diferencia de /api/messages, NO marca el canal como leído:
 * saltar a un mensaje antiguo no debe borrar los no leídos posteriores.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const channelId = ctx.url.searchParams.get('channelId');
  const message = ctx.url.searchParams.get('message');
  if (!channelId) return json({ error: 'missing_channelId' }, 400);
  if (!message) return json({ error: 'missing_message' }, 400);
  const limitRaw = ctx.url.searchParams.get('limit');
  const limit = limitRaw ? Math.min(Math.max(Number(limitRaw), 1), 100) : 50;

  try {
    const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
    const { messages, threadRootId } = await client.messagesAround(channelId, message, limit);
    // `threadRootId` solo viene si el objetivo era una respuesta de hilo.
    return json(threadRootId ? { messages, threadRootId } : { messages });
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
