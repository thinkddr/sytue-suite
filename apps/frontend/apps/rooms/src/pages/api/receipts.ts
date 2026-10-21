/**
 * GET /api/receipts?channelId= — acuse de lectura de los otros miembros de un DM
 * («Visto», Ola 16), proxy BFF hacia el servicio Go Sytue Chat. Vacío si el canal no
 * es un DM (el backend solo calcula receipts para directos, paridad Mattermost).
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const channelId = ctx.url.searchParams.get('channelId');
  if (!channelId) return json({ error: 'missing_channelId' }, 400);

  try {
    const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
    const receipts = await client.dmReceipts(channelId);
    return json({ receipts });
  } catch (err) {
    return json({ error: 'chat_error', message: String(err) }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
