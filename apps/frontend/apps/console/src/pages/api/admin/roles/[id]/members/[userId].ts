/*
 * BFF de Sytue Admin — quitar un rol a un usuario.
 *
 *   DELETE /api/admin/roles/:id/members/:userId   Quita el rol al usuario.
 *
 * Proxya a Sytue ID (DELETE /api/v1/admin/roles/:id/members/:userId, Go) con el access
 * token httpOnly; exige rol admin/owner. Propaga el status del servicio (403/404).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../../server/bff.js';
import { proxyStatus, unassignCustomRole } from '../../../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const DELETE: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  const id = ctx.params.id;
  const userId = ctx.params.userId;
  if (!id || !userId) return json({ error: 'bad_request' }, 400);
  try {
    await unassignCustomRole(guard.session, id, userId);
    return json({ id, user_id: userId, removed: true });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
