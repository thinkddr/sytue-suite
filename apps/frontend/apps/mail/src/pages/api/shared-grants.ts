/**
 * /api/shared-grants — gestión del ACL de un buzón compartido. Proxy BFF al
 * servicio Go (REST /api/v1/shared-mailboxes/{accountId}/grants), que EXIGE
 * permiso `manage` sobre la cuenta. GET lista el ACL (accountId en query); POST
 * multiplexa por la forma del cuerpo (el checkOrigin CSRF de Astro rechaza
 * PUT/DELETE del navegador):
 *   { accountId, grantee, permission } → conceder/actualizar
 *   { accountId, grantee, delete:true } → revocar
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, mailPost } from '../../lib/mailrest.js';

export const prerender = false;

const UUID = /^[0-9a-f-]{36}$/i;
const PERMS = new Set(['read', 'send', 'manage']);

interface RawGrant {
  grantee?: string;
  permission?: string;
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const accountId = ctx.url.searchParams.get('accountId') ?? '';
  if (!UUID.test(accountId)) return json({ error: 'invalid_account' }, 400);
  try {
    const res = await mailGet(session.accessToken, `/api/v1/shared-mailboxes/${accountId}/grants`);
    if (res.status === 403) return json({ error: 'forbidden' }, 403);
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as { grants?: RawGrant[] };
    return json({ grants: (body.grants ?? []).map(mapGrant) });
  } catch (err) {
    return json({ error: 'mail_error', message: String(err) }, 502);
  }
};

interface Payload {
  accountId?: string;
  grantee?: string;
  permission?: string;
  delete?: boolean;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as Payload;
  if (!body.accountId || !UUID.test(body.accountId)) return json({ error: 'invalid_account' }, 400);
  if (!body.grantee || !body.grantee.trim()) return json({ error: 'invalid_payload' }, 400);
  const path = `/api/v1/shared-mailboxes/${body.accountId}/grants`;
  const payload = body.delete
    ? { grantee: body.grantee.trim(), delete: true }
    : { grantee: body.grantee.trim(), permission: body.permission };
  if (!body.delete && !PERMS.has(String(body.permission)))
    return json({ error: 'invalid_permission' }, 400);
  try {
    const res = await mailPost(session.accessToken, path, payload);
    if (res.status === 403) return json({ error: 'forbidden' }, 403);
    if (!res.ok)
      return json({ error: 'mail_error', status: res.status }, res.status === 400 ? 400 : 502);
    return json({ ok: true });
  } catch (err) {
    return json({ error: 'mail_error', message: String(err) }, 502);
  }
};

function mapGrant(g: RawGrant) {
  return { grantee: g.grantee ?? '', permission: g.permission ?? 'read' };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
