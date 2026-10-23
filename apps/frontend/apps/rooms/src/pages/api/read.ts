/**
 * POST /api/read — marca un canal como leído por el usuario hasta su último
 * mensaje (proxy BFF hacia el servicio Go Sytue Chat). Cuerpo: { channelId }.
 * Pone a cero los contadores durables de no leídos y menciones del canal. JSON
 * (no form) para no disparar la protección CSRF de Astro dev.
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
    await chatClient({ accessToken: session.accessToken, userId: session.user.id }).markRead(
      channelId,
    );
    return json({ ok: true });
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
