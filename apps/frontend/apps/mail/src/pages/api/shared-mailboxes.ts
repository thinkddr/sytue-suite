/**
 * /api/shared-mailboxes — buzones compartidos/delegados accesibles + creación de
 * un buzón compartido. Proxy BFF al servicio Go (REST /api/v1/shared-mailboxes).
 * GET lista los buzones a los que tengo acceso (con mi permiso); POST crea un
 * buzón compartido de la organización (el creador queda como `manage`).
 * Autorizado por cookie de sesión; el ACL y la validación de dominio los impone
 * el servicio.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, mailPost } from '../../lib/mailrest.js';

export const prerender = false;

interface RawShared {
  account_id: string;
  address?: string;
  display_name?: string;
  kind?: string;
  permission?: string;
  edit_requested?: boolean;
}

function mapShared(m: RawShared) {
  return {
    accountId: m.account_id,
    address: m.address ?? '',
    displayName: m.display_name ?? '',
    kind: m.kind === 'user' ? 'user' : 'shared',
    permission: m.permission ?? 'read',
    editRequested: m.edit_requested === true,
  };
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await mailGet(session.accessToken, '/api/v1/shared-mailboxes');
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as { shared_mailboxes?: RawShared[] };
    return json({ sharedMailboxes: (body.shared_mailboxes ?? []).map(mapShared) });
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};

interface Payload {
  address?: string;
  displayName?: string;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as Payload;
  if (typeof body.address !== 'string' || !body.address.trim())
    return json({ error: 'invalid_payload' }, 400);
  try {
    const res = await mailPost(session.accessToken, '/api/v1/shared-mailboxes', {
      address: body.address.trim(),
      display_name: (body.displayName ?? '').trim(),
    });
    if (!res.ok)
      return json({ error: 'mail_error', status: res.status }, res.status === 409 ? 409 : 502);
    return json({ mailbox: mapShared((await res.json()) as RawShared) }, 201);
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
