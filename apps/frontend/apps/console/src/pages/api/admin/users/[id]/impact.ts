/*
 * BFF de Sytue Admin — impacto de una acción sobre un usuario.
 *
 *   GET /api/admin/users/:id/impact -> { active_sessions, groups, last_owner }
 *
 * Es lo que la ficha consulta ANTES de enseñar una confirmación. Solo LEE, y
 * exige lo mismo que el resto de la ficha: no revela nada que un admin de la
 * organización no pueda ver ya.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { loadUserImpact, proxyStatus } from '../../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['users.read', 'users.manage'],
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
    return json(await loadUserImpact(guard.session, id));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
