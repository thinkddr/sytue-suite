/**
 * GET /api/thread?messageId= — hilo de un mensaje raíz (proxy BFF hacia el
 * servicio Go Sytue Chat). Devuelve { root, replies } ya en el shape del UI.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const messageId = ctx.url.searchParams.get('messageId');
  if (!messageId) return json({ error: 'missing_messageId' }, 400);

  try {
    const thread = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).getThread(messageId);
    return json(thread);
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
