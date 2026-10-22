/**
 * POST /api/card-vote — marca o retira un valor de la tarjeta de un mensaje (proxy
 * BFF hacia el servicio Go): votar una opción de la encuesta o dar por hecha una
 * tarea. Cuerpo: { messageId, value, add }. Devuelve el recuento YA agregado, para
 * que quien vota vea el resultado con la respuesta; el resto de clientes lo recibe
 * por WebSocket. JSON (no form) para no disparar la protección CSRF de Astro dev.
 *
 * Qué valores existen lo decide el servicio contra la tarjeta guardada: aquí no se
 * valida el contenido de `value`, solo que venga.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { messageId?: string; value?: string; add?: boolean };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const messageId = payload.messageId?.trim();
  const value = payload.value?.trim();
  if (!messageId) return json({ error: 'missing_messageId' }, 400);
  if (!value) return json({ error: 'missing_value' }, 400);

  try {
    const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
    const votes = await client.voteCard(messageId, value, payload.add !== false);
    return json({ votes });
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
