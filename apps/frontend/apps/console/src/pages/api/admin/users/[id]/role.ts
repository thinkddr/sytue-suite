/*
 * BFF de Sytue Admin — cambiar el rol de un miembro.
 *
 *   POST /api/admin/users/:id/role   body { role: SytueRole }
 *       Cambia el rol del usuario en la org activa. Una degradación revoca sus
 *       sesiones. Rol admin/owner.
 *
 * Proxya a Sytue ID (POST /api/v1/admin/users/:id/role, Go) con el access token
 * httpOnly; propaga el status del servicio (403/404/409/400) — incluido el guard
 * del último admin (409). El token nunca sale al cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { setMemberRole, proxyStatus } from '../../../../../lib/admin.js';
import type { SytueRole } from '../../../../../lib/types.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

const ROLES: readonly SytueRole[] = ['owner', 'admin', 'member', 'billing'];

export const POST: APIRoute = async (ctx) => {
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

  const body = (await ctx.request.json().catch(() => null)) as { role?: unknown } | null;
  const role = body?.role;
  if (typeof role !== 'string' || !ROLES.includes(role as SytueRole)) {
    return json({ error: 'bad_request', message: 'Rol inválido.' }, 400);
  }
  try {
    return json(await setMemberRole(guard.session, id, role as SytueRole));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
