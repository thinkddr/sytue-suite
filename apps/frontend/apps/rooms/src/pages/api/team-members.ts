/**
 * POST /api/team-members — BAJA del team (proxy BFF hacia el servicio Go Sytue Chat).
 *   POST { subject, remove: true } → { ok: true }
 *
 * El ALTA vive en /api/invite (la despacha «/invite @usuario»); esta es su
 * simétrica, y sin ella una baja no se podía completar desde el producto: sacar a
 * alguien de un canal lo dejaba igualmente en el team, viendo los canales públicos
 * y apareciendo en el directorio de menciones.
 *
 * Por POST y no por DELETE del navegador, como /api/channel-members: así no dispara
 * la protección CSRF de Astro dev; el proxy hace el verbo real. Sin lógica de
 * negocio: la AUTORIZACIÓN (rol admin de team) la aplica el servicio Go.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient, ChatHttpError } from '../../lib/chat.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { subject?: string; remove?: boolean };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  const subject = payload.subject?.trim();
  if (!subject) return json({ error: 'missing_subject' }, 400);
  if (payload.remove !== true) return json({ error: 'unsupported' }, 400);

  try {
    const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
    await client.removeTeamMember(subject);
    return json({ ok: true });
  } catch (err) {
    // Los 4xx se PRESERVAN (403 si no eres admin, 404 si ya no está): el aviso del
    // composer distingue «no puedes» de «se cayó algo».
    if (err instanceof ChatHttpError && err.status >= 400 && err.status < 500) {
      return json({ error: 'chat_error' }, err.status);
    }
    return json({ error: 'chat_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
