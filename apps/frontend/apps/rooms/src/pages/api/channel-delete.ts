/**
 * POST /api/channel-delete — borra un canal o, en un mensaje directo, «borra el chat»
 * solo para quien lo pide (proxy BFF hacia el servicio Go de Sytue Rooms).
 *   { channelId }               → DELETE /channels/{id} (admin del canal o del equipo)
 *   { channelId, direct: true } → POST /channels/{id}/hide (solo para ti)
 * POST con JSON (no DELETE del navegador) para no disparar la protección CSRF de
 * Astro; el proxy hace el verbo real. La autorización la aplica el servicio, y sus
 * 4xx se preservan para que la isla elija el aviso.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient, chatFail } from '../../lib/chat.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { channelId?: string; direct?: boolean };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  const channelId = payload.channelId?.trim();
  if (!channelId) return json({ error: 'missing_channelId' }, 400);

  try {
    const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
    if (payload.direct === true) await client.hideDirect(channelId);
    else await client.deleteChannel(channelId);
    return json({ ok: true });
  } catch (err) {
    return chatFail(err);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
