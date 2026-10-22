/*
 * BFF de Sytue Admin — revocar un dispositivo de la organización (vista admin).
 *
 *   DELETE /api/admin/devices/:id   Revoca (borra) el dispositivo de la org activa.
 *
 * Proxya a Sytue ID (DELETE /api/v1/admin/devices/:id, Go) con el access token httpOnly.
 * Rol admin/owner de ESA org (cross-org ⇒ 404).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { adminRevokeDevice, proxyStatus } from '../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

function guardStatus(s: number): Response {
  return json({ error: s === 403 ? 'forbidden' : 'unauthenticated' }, s === 403 ? 403 : 401);
}

export const DELETE: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['users.manage'],
  });
  if (guard instanceof Response) return guardStatus(guard.status);
  const id = ctx.params.id ?? '';
  try {
    await adminRevokeDevice(guard.session, id);
    return json({ id, deleted: true });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
