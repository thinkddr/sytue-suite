/*
 * BFF de Sytue Admin — recálculo de la membresía de un grupo dinámico.
 *
 *   POST /api/admin/groups/:id/recompute
 *     → { added, removed, member_count }  (efecto REAL del recálculo)
 *
 * Proxya a Sytue ID (POST /api/v1/admin/groups/:id/recompute, Go) con el access token
 * httpOnly; exige rol admin/owner y groups.manage. Sytue ID responde 400 si el grupo es
 * manual (no hay nada que calcular) y 404 si es de otro tenant: el status se propaga vía
 * proxyStatus. El token nunca sale al cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { proxyStatus, recomputeDynamicGroup } from '../../../../../lib/admin.js';

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
    perms: ['groups.manage'],
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
    const res = await recomputeDynamicGroup(guard.session, id);
    return json({ added: res.added, removed: res.removed, member_count: res.memberCount });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
