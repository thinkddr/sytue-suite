/**
 * GET /api/browse?q= — canales públicos del team a los que el usuario NO
 * pertenece, para «explorar canales» (proxy BFF hacia el servicio Go Sytue Chat).
 * El access token no toca el navegador.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const q = ctx.url.searchParams.get('q') ?? '';
  try {
    const channels = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).browseChannels(q);
    return json({ channels });
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
