/**
 * /api/invitations — invitaciones de OTRAS organizaciones a un canal, un DM o un grupo:
 * la gente de fuera acepta antes de entrar (decidido por el dueño el 2026-10-16).
 *   GET                                         → { invitations: [...] }
 *   POST { channelId, action: 'accept' | 'reject' } → { ok: true }
 * Proxy BFF puro: quién está invitado a qué lo decide el servicio Go con el token.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient, ChatHttpError } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
    return json({ invitations: await client.invitations() });
  } catch (err) {
    return json({ error: 'chat_error' }, 502);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  let payload: { channelId?: string; action?: string };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  const channelId = payload.channelId?.trim();
  if (!channelId || (payload.action !== 'accept' && payload.action !== 'reject')) {
    return json({ error: 'bad_request' }, 400);
  }
  try {
    const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
    await client.answerInvitation(channelId, payload.action === 'accept');
    return json({ ok: true });
  } catch (err) {
    if (err instanceof ChatHttpError) return json({ error: 'chat_error' }, err.status);
    return json({ error: 'chat_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
