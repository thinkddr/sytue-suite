/**
 * GET /api/unfurl?url= — vista previa (OpenGraph) de una URL http/https (Ola 16),
 * proxy BFF hacia el servicio Go Sytue Chat. El backend la obtiene UNA vez (SSRF-safe)
 * y la cachea por tenant; aquí solo se reenvía con el token federado (que no toca el
 * navegador). Se cachea en el navegador un rato para no repetir la llamada al re-render.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const url = (ctx.url.searchParams.get('url') ?? '').trim();
  if (!url) return json({ error: 'missing_url' }, 400);

  try {
    const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
    const unfurl = await client.unfurl(url);
    return json({ unfurl }, 200, 'private, max-age=300');
  } catch (err) {
    return json({ error: 'chat_error', message: String(err) }, 502);
  }
};

function json(body: unknown, status = 200, cacheControl = 'no-store'): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': cacheControl },
  });
}
