/**
 * GET /api/saved?before= — mis mensajes guardados, cross-canal (proxy BFF hacia el
 * servicio Go Sytue Chat). Paginación keyset por fecha de guardado: `before` es el
 * cursor de la página anterior (nextBefore). Alimenta el panel «Guardados».
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const opts: { before?: string; limit?: number } = { limit: 50 };
  const before = ctx.url.searchParams.get('before');
  if (before) opts.before = before;

  try {
    const data = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).listSaved(opts);
    return json(data);
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
