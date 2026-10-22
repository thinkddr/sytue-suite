/**
 * POST /api/join — une al usuario a un canal público (proxy BFF hacia el servicio
 * Go Sytue Chat). Cuerpo: { channelId }. Devuelve { channel }.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { channelId?: string };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const channelId = payload.channelId?.trim();
  if (!channelId) return json({ error: 'missing_channelId' }, 400);

  try {
    const channel = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).joinChannel(channelId);
    return json({ channel });
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
