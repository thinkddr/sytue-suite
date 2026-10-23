/**
 * /api/emoji — emoji personalizados del tenant (Ola 14), proxy BFF hacia el
 * servicio Go Sytue Chat.
 *   GET                          → { emoji: [...] }
 *   POST { name, fileId }        → crea; devuelve { emoji }
 *   POST { name, delete: true }  → borra (DELETE server-to-server; el POST evita el
 *                                  CSRF de Astro, como pins/categorías)
 * La imagen del emoji se sube antes a Sytue Drive por /api/upload; aquí solo se liga
 * el shortcode al fileId. El servicio Go valida que sea una imagen accesible.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
    return json({ emoji: await client.listCustomEmoji(ctx.request.signal) });
  } catch (err) {
    return json({ error: 'chat_error' }, 502);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { name?: string; fileId?: string; delete?: boolean };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  const name = payload.name?.trim();
  if (!name) return json({ error: 'missing_name' }, 400);

  try {
    const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
    if (payload.delete === true) {
      await client.deleteCustomEmoji(name);
      return json({ ok: true });
    }
    const fileId = payload.fileId?.trim();
    if (!fileId) return json({ error: 'missing_fileId' }, 400);
    return json({ emoji: await client.createCustomEmoji(name, fileId, ctx.request.signal) }, 201);
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
