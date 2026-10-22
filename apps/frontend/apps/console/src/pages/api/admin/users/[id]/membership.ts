/*
 * BFF de Sytue Admin — quitar a un miembro de la organización.
 *
 *   DELETE /api/admin/users/:id/membership   -> { id, sessions_revoked }
 *       Quita al usuario de la org activa y revoca sus sesiones. No borra la
 *       cuenta (puede pertenecer a otras orgs). Rol admin/owner.
 *
 * Proxya a Sytue ID (DELETE /api/v1/admin/users/:id/membership, Go) con el access
 * token httpOnly; propaga el status del servicio (403/404/409, incluido el guard
 * del último admin). El token nunca sale al cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { removeMember, proxyError, proxyStatus } from '../../../../../lib/admin.js';

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
    perms: ['users.manage'],
  });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  const id = ctx.params.id;
  if (!id) return json({ error: 'bad_request' }, 400);
  try {
    return json(await removeMember(guard.session, id));
  } catch (e) {
    // Con el `code` de Sytue ID: «insufficient_user_authentication» lleva a confirmar
    // la identidad, y «sole_owner» o «last_admin» dicen qué corregir.
    return json(proxyError(e), proxyStatus(e));
  }
};
