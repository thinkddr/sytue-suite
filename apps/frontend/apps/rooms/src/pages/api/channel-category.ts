/**
 * POST /api/channel-category — asigna un canal a una categoría del sidebar del usuario
 * (proxy BFF hacia el servicio Go Sytue Chat). Cuerpo: { channelId, categoryId?,
 * sortOrder? }. `categoryId` vacío/ausente/null = categoría por defecto según kind.
 * Devuelve { channel } con la categoría/orden resueltos. JSON (no form) contra el CSRF.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { channelId?: string; categoryId?: string | null; sortOrder?: number };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const channelId = payload.channelId?.trim();
  if (!channelId) return json({ error: 'missing_channelId' }, 400);
  const categoryId = payload.categoryId?.trim() ? payload.categoryId.trim() : null;

  try {
    const channel = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).setChannelCategory(channelId, categoryId, payload.sortOrder);
    return json({ channel });
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
