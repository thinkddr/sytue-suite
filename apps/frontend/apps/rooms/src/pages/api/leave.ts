/**
 * POST /api/leave — abandona un canal (proxy BFF hacia el servicio Go Sytue Chat).
 * Cuerpo: { channelId }. JSON (no form) para no disparar la protección CSRF de Astro.
 * Lo usan el slash command «/leave» y «Salir del canal» de los menús. Los 4xx del
 * servicio se preservan (409 = último admin de un canal privado), como en
 * channel-members.ts.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient, chatFail } from '../../lib/chat.js';

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
    const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
    await client.leaveChannel(channelId);
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
