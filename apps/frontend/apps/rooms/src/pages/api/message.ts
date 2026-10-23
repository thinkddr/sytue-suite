/**
 * POST /api/message — edita o borra un mensaje propio (proxy BFF hacia el servicio
 * Go Sytue Chat). Cuerpo: { messageId, body } para editar, o { messageId, delete:true }
 * para borrar. El resto de clientes recibe el cambio por WebSocket (message_edited /
 * message_deleted). JSON (no form) para no disparar la protección CSRF de Astro dev.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { reference } from '../../client/matrix/types.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { matrix_event?: unknown; messageId?: string; body?: string; delete?: boolean };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const messageId = payload.messageId?.trim();
  if (!messageId) return json({ error: 'missing_messageId' }, 400);

  const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });

  if (payload.delete === true) {
    try {
      await client.deleteMessage(messageId);
      return json({ ok: true });
    } catch {
      return json({ error: 'chat_error' }, 502);
    }
  }

  const matrixEvent = reference(payload.matrix_event);
  const body = payload.body?.trim() ?? '';
  if (matrixEvent && body) return json({ error: 'plaintext_not_allowed' }, 400);
  if (!body && !matrixEvent) return json({ error: 'empty_body' }, 400);
  if (body.length > 8000) return json({ error: 'body_too_long' }, 413);
  try {
    const message = await client.editMessage(messageId, body, matrixEvent);
    return json({ message });
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
