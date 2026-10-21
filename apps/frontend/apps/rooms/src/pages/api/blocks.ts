/**
 * /api/blocks — personas bloqueadas de quien tiene la sesión (proxy BFF hacia el
 * servicio Go; el access token no toca el navegador).
 *   GET                              → { blocks: [...] }
 *   POST { channelId, subject }      → bloquea a alguien del canal; { block }
 *   POST { subject, delete: true }   → desbloquea; { ok: true }
 *
 * Quién bloquea es SIEMPRE el de la sesión: lo pone el servicio a partir del
 * token. Todo por POST, como /api/pin, para no disparar la protección CSRF.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient, chatFail } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const blocks = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).listBlocks();
    return json({ blocks });
  } catch (err) {
    return chatFail(err);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { channelId?: string; subject?: string; delete?: boolean };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  const subject = payload.subject?.trim();
  if (!subject) return json({ error: 'missing_subject' }, 400);

  const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
  try {
    if (payload.delete === true) {
      await client.unblock(subject);
      return json({ ok: true });
    }
    const channelId = payload.channelId?.trim();
    if (!channelId) return json({ error: 'missing_channelId' }, 400);
    return json({ block: await client.blockMember(channelId, subject) });
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
