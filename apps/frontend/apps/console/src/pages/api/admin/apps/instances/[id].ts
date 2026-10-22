/*
 * BFF de Sytue Admin — una instancia de integración.
 *
 *   DELETE /api/admin/apps/instances/:id   Borra la instancia (y su secreto en cascada).
 *
 * Proxya al gateway (DELETE /api/v1/instances/:id, Go) con el access token httpOnly;
 * propaga el status. Rol admin/owner. Puro passthrough.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { deleteInstance, proxyStatus } from '../../../../../lib/gateway.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const DELETE: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['apps.manage'],
  });
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);
  try {
    await deleteInstance(guard.session, ctx.params.id ?? '');
    return json({ deleted: true });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
