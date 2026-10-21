/*
 * BFF de Sytue Admin — estado de ciclo de vida de un usuario.
 *
 *   POST /api/admin/users/:id/status   body { status: 'suspended' | 'active' }
 *       Suspende o reactiva la cuenta. Suspender revoca además sus sesiones.
 *
 * Proxya a Sytue ID (POST /api/v1/admin/users/:id/{suspend|reactivate}, Go) con
 * el access token httpOnly; exige rol admin/owner. Propaga el status del servicio
 * (403/404/409) para que la UI muestre el motivo real. El token nunca sale al
 * cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { setUserStatus, proxyStatus } from '../../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

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

  const body = (await ctx.request.json().catch(() => null)) as { status?: unknown } | null;
  const status = body?.status;
  if (status !== 'suspended' && status !== 'active') {
    return json({ error: 'bad_request', message: 'status debe ser "suspended" o "active".' }, 400);
  }
  try {
    return json(await setUserStatus(guard.session, id, status));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
