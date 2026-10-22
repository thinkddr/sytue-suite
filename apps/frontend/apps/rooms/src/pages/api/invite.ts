/**
 * POST /api/invite — añade a un miembro al team del usuario (proxy BFF hacia el
 * servicio Go Sytue Chat). Cuerpo: { subject, name }. El backend exige rol admin.
 * Lo despacha el slash command «/invite @usuario» del composer (el sub se resuelve
 * en el cliente contra el roster del team).
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { subject?: string; name?: string };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  const subject = payload.subject?.trim();
  if (!subject) return json({ error: 'missing_subject' }, 400);

  try {
    const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
    await client.addTeamMember(subject, payload.name?.trim() ?? '');
    return json({ ok: true });
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
