/**
 * POST /api/mute { threadId, muted } — silenciar o dejar de silenciar una
 * conversación, proxy BFF al servicio Go (PUT /api/v1/threads/{id}/mute). Siempre
 * POST: el checkOrigin CSRF de Astro rechaza PUT desde el navegador. Devuelve el
 * estado y cuántos mensajes salieron de Recibidos al silenciar.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailPut } from '../../lib/mailrest.js';
import { json, rechazoDelServicio } from '../../lib/bffRespuestas.js';

export const prerender = false;

const UUID = /^[0-9a-f-]{36}$/i;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as {
    threadId?: string;
    muted?: boolean;
  };
  if (typeof body.threadId !== 'string' || !UUID.test(body.threadId)) {
    return json({ error: 'invalid_thread' }, 400);
  }
  if (typeof body.muted !== 'boolean') return json({ error: 'invalid_payload' }, 400);
  try {
    const res = await mailPut(session.accessToken, `/api/v1/threads/${body.threadId}/mute`, {
      muted: body.muted,
    });
    if (!res.ok) return rechazoDelServicio(res);
    const out = (await res.json()) as { muted?: boolean; archived?: number };
    return json({ muted: out.muted ?? body.muted, archived: out.archived ?? 0 });
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};
