/**
 * POST /api/reactions — añade o retira una reacción de un mensaje (proxy BFF hacia
 * el servicio Go Sytue Chat). Cuerpo: { messageId, emoji, add }. El resto de
 * clientes recibe el cambio por WebSocket. JSON (no form) para no disparar la
 * protección CSRF de Astro dev.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { messageId?: string; emoji?: string; add?: boolean };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const messageId = payload.messageId?.trim();
  const emoji = payload.emoji?.trim();
  if (!messageId) return json({ error: 'missing_messageId' }, 400);
  if (!emoji) return json({ error: 'missing_emoji' }, 400);

  try {
    const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
    if (payload.add === false) await client.removeReaction(messageId, emoji);
    else await client.addReaction(messageId, emoji);
    return json({ ok: true });
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
