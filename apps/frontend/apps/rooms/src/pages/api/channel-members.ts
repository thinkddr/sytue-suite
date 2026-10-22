/**
 * /api/channel-members — roster de un canal (proxy BFF autorizado por la cookie de
 * sesión httpOnly hacia el servicio Go Sytue Chat). El access token no toca el
 * navegador.
 *   GET  ?channelId=…                          → { members: [...] }
 *   POST { channelId, subject, name? }         → añade; devuelve { member } con 201
 *   POST { channelId, subject, role }          → fija el rol de canal; { member }
 *   POST { channelId, subject, remove: true }  → expulsa (o sales tú); { ok: true }
 *
 * Todo por POST (no DELETE/PATCH del navegador) para no disparar la protección CSRF
 * de Astro dev, como ya hacen /api/pin y /api/channel; el proxy hace el verbo real.
 * Sin lógica de negocio: la AUTORIZACIÓN la aplica el servicio Go (admin de canal o
 * de team; el DELETE admite además autoservicio) y también la verja de que el
 * añadido pertenezca al team.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient, chatFail } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const channelId = new URL(ctx.request.url).searchParams.get('channelId')?.trim();
  if (!channelId) return json({ error: 'missing_channelId' }, 400);

  try {
    const members = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).listChannelMembers(channelId);
    return json({ members });
  } catch (err) {
    return chatFail(err);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: {
    channelId?: string;
    subject?: string;
    name?: string;
    role?: string;
    remove?: boolean;
  };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const channelId = payload.channelId?.trim();
  const subject = payload.subject?.trim();
  if (!channelId) return json({ error: 'missing_channelId' }, 400);
  if (!subject) return json({ error: 'missing_subject' }, 400);

  const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
  try {
    if (payload.remove === true) {
      await client.removeChannelMember(channelId, subject);
      return json({ ok: true });
    }
    if (payload.role !== undefined) {
      // Verja del rol: el CHECK de la BD solo admite estos dos.
      if (payload.role !== 'admin' && payload.role !== 'member') {
        return json({ error: 'invalid_role' }, 400);
      }
      const member = await client.setChannelMemberRole(channelId, subject, payload.role);
      return json({ member });
    }
    const member = await client.addChannelMember(channelId, subject, payload.name?.trim());
    return json({ member }, 201);
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
