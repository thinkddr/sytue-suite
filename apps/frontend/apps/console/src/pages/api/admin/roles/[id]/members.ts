/*
 * BFF de Sytue Admin — asignar un rol a un usuario.
 *
 *   POST /api/admin/roles/:id/members  body { user_id }   Asigna el rol al usuario.
 *
 * Proxya a Sytue ID (POST /api/v1/admin/roles/:id/members, Go) con el access token
 * httpOnly; exige rol admin/owner. Propaga el status del servicio (400/403/404/409).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { assignCustomRole, proxyStatus } from '../../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  const id = ctx.params.id;
  if (!id) return json({ error: 'bad_request' }, 400);
  const body = (await ctx.request.json().catch(() => null)) as { user_id?: unknown } | null;
  const userId = typeof body?.user_id === 'string' ? body.user_id.trim() : '';
  if (!userId) return json({ error: 'bad_request', message: 'user_id es obligatorio.' }, 400);
  try {
    return json(await assignCustomRole(guard.session, id, userId), 201);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
