/**
 * POST /api/ack — acuse de recibo de un mensaje que lo pide (Ola 12; proxy BFF hacia
 * el servicio Go Sytue Chat). Cuerpo: { messageId }. Idempotente. Devuelve el nuevo
 * total «Confirmado por N». JSON (no form) contra el CSRF de Astro dev.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { messageId?: string };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const messageId = payload.messageId?.trim();
  if (!messageId) return json({ error: 'missing_messageId' }, 400);

  try {
    const count = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).ackMessage(messageId);
    return json({ count });
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
