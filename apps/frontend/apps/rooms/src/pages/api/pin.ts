/**
 * POST /api/pin — fija o desfija un mensaje (proxy BFF hacia el servicio Go Sytue
 * Chat). Cuerpo: { messageId } para fijar (devuelve { message }), o
 * { messageId, delete:true } para desfijar. El resto de clientes recibe el cambio
 * por WebSocket (evento pin). JSON (no form) y el borrado vía POST {delete:true}
 * para no disparar la protección CSRF de Astro dev (rechaza DELETE del navegador).
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { messageId?: string; delete?: boolean };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const messageId = payload.messageId?.trim();
  if (!messageId) return json({ error: 'missing_messageId' }, 400);

  const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
  try {
    if (payload.delete === true) {
      await client.unpinMessage(messageId);
      return json({ ok: true });
    }
    const message = await client.pinMessage(messageId);
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
