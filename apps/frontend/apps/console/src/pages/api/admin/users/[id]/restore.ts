/*
 * BFF de Sytue Admin — restaurar una cuenta cerrada.
 *
 *   POST /api/admin/users/:id/restore -> { id, status: 'suspended', note }
 *
 * Proxya a Sytue ID (POST /api/v1/admin/users/:id/restore). La cuenta vuelve
 * SUSPENDIDA: restaurar devuelve los datos, reactivar devuelve el acceso.
 *
 * Exige users.manage y, en Sytue ID, autenticación reciente (step-up): resucita
 * datos de una cuenta apagada. Su 401 `insufficient_user_authentication` viaja
 * con el `code` para que la pantalla lleve a confirmar la identidad y vuelva.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { restoreUser, proxyError, proxyStatus } from '../../../../../lib/admin.js';

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
  try {
    return json(await restoreUser(guard.session, id));
  } catch (e) {
    return json(proxyError(e), proxyStatus(e));
  }
};
