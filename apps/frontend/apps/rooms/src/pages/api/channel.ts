/**
 * POST /api/channel — actualiza un canal (tema / propósito / archivar / renombrar;
 * solo admin, lo valida el servicio Go) vía proxy BFF. Cuerpo: { channelId, topic?,
 * purpose?, archived?, name? }. Devuelve { channel }. POST (no PATCH del navegador) para no
 * disparar la protección CSRF de Astro dev; el proxy hace el PATCH al backend.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: {
    channelId?: string;
    topic?: string;
    purpose?: string;
    archived?: boolean;
    name?: string;
  };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const channelId = payload.channelId?.trim();
  if (!channelId) return json({ error: 'missing_channelId' }, 400);
  if (
    payload.topic === undefined &&
    payload.purpose === undefined &&
    payload.archived === undefined &&
    payload.name === undefined
  ) {
    return json({ error: 'nothing_to_update' }, 400);
  }

  try {
    const channel = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).updateChannel(channelId, {
      ...(payload.topic !== undefined ? { topic: payload.topic } : {}),
      ...(payload.purpose !== undefined ? { purpose: payload.purpose } : {}),
      ...(payload.archived !== undefined ? { archived: payload.archived } : {}),
      ...(payload.name !== undefined ? { name: payload.name } : {}),
    });
    return json({ channel });
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
