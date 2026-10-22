/**
 * POST /api/prefs — fija favorito, silenciado y/o nivel de notificación de un canal
 * para el usuario (proxy BFF hacia el servicio Go Sytue Chat). Cuerpo: { channelId,
 * favorite?, muted?, notifyLevel? }. notifyLevel ('all'|'mentions'|'none') es la
 * política de aviso (Ola 13); muted es su proyección legada. Devuelve { channel } con
 * las preferencias resueltas. JSON (no form) para no disparar el CSRF de Astro dev.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

const LEVELS = ['all', 'mentions', 'none'] as const;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: {
    channelId?: string;
    favorite?: boolean;
    muted?: boolean;
    notifyLevel?: string;
  };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const channelId = payload.channelId?.trim();
  if (!channelId) return json({ error: 'missing_channelId' }, 400);
  const notifyLevel = LEVELS.find((l) => l === payload.notifyLevel);
  if (payload.notifyLevel !== undefined && !notifyLevel) {
    return json({ error: 'invalid_notify_level' }, 400);
  }
  if (payload.favorite === undefined && payload.muted === undefined && notifyLevel === undefined) {
    return json({ error: 'nothing_to_update' }, 400);
  }

  try {
    const channel = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).setChannelPrefs(channelId, {
      ...(payload.favorite !== undefined ? { favorite: payload.favorite } : {}),
      ...(payload.muted !== undefined ? { muted: payload.muted } : {}),
      ...(notifyLevel !== undefined ? { notifyLevel } : {}),
    });
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
