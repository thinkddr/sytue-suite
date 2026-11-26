/*
 * BFF de Sytue Admin — acceso efectivo de una persona.
 *
 *   GET /api/admin/users/:id/effective-access
 *       -> { role, permissions[{permission, source, detail?, expires_at?}],
 *            custom_roles[], groups[], pending_elevations[], as_of }
 *
 * Proxya a Sytue ID (GET /api/v1/admin/users/:id/effective-access). Solo LEE, con el
 * mismo listón que el resto de la ficha (users.read o users.manage, como el
 * directorio). El resultado lo calcula Sytue ID con la regla con la que AUTORIZA:
 * la consola no suma nada por su cuenta, para que lo que se enseña y lo que se
 * aplica no puedan divergir.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { loadEffectiveAccess, proxyStatus } from '../../../../../lib/admin.js';

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
    return json(await loadEffectiveAccess(guard.session, id));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
