/**
 * GET /api/browse?q= — canales públicos del team a los que el usuario NO
 * pertenece, para «explorar canales» (proxy BFF hacia el servicio Go Sytue Chat).
 * GET /api/browse?archived=1 — los canales ARCHIVADOS suyos, la otra pestaña.
 * El access token no toca el navegador.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient, chatFail } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const q = ctx.url.searchParams.get('q') ?? '';
  const archivados = ctx.url.searchParams.get('archived') === '1';
  try {
    const chat = chatClient({ accessToken: session.accessToken, userId: session.user.id });
    const channels = archivados ? await chat.listArchivedChannels() : await chat.browseChannels(q);
    return json({ channels });
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
