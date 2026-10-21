/**
 * /api/delegation — delegación de MI PROPIO buzón (asistente ↔ directivo). Proxy
 * BFF al servicio Go (REST /api/v1/delegation), que opera sobre la cuenta del
 * usuario autenticado (dueño ⇒ `manage` implícito). GET lista a quién he delegado;
 * POST multiplexa: { grantee, permission } concede, { grantee, delete:true } revoca.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, mailPost } from '../../lib/mailrest.js';

export const prerender = false;

const PERMS = new Set(['read', 'send', 'manage']);

interface RawGrant {
  grantee?: string;
  permission?: string;
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await mailGet(session.accessToken, '/api/v1/delegation');
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as { grants?: RawGrant[] };
    return json({
      grants: (body.grants ?? []).map((g) => ({
        grantee: g.grantee ?? '',
        permission: g.permission ?? 'read',
      })),
    });
  } catch (err) {
    return json({ error: 'mail_error', message: String(err) }, 502);
  }
};

interface Payload {
  grantee?: string;
  permission?: string;
  delete?: boolean;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as Payload;
  if (!body.grantee || !body.grantee.trim()) return json({ error: 'invalid_payload' }, 400);
  const payload = body.delete
    ? { grantee: body.grantee.trim(), delete: true }
    : { grantee: body.grantee.trim(), permission: body.permission };
  if (!body.delete && !PERMS.has(String(body.permission)))
    return json({ error: 'invalid_permission' }, 400);
  try {
    const res = await mailPost(session.accessToken, '/api/v1/delegation', payload);
    if (!res.ok)
      return json({ error: 'mail_error', status: res.status }, res.status === 400 ? 400 : 502);
    return json({ ok: true });
  } catch (err) {
    return json({ error: 'mail_error', message: String(err) }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
