/*
 * BFF de Sytue Admin — quitar un alias de correo.
 *
 *   DELETE /api/admin/mailboxes/{id}
 *
 * Proxya a Sytue ID (DELETE /api/v1/admin/mailboxes/{id}, Go). Sytue ID se niega a
 * borrar una dirección PRINCIPAL: quitarle a alguien su dirección lo dejaría sin
 * poder recibir, y eso no puede ser el efecto de pulsar «eliminar» en una lista de
 * alias. Rol admin/owner; el token httpOnly nunca sale al cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { deleteMailboxAlias, proxyStatus } from '../../../../lib/admin.js';

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
  const id = ctx.params.id ?? '';
  if (!id) return json({ error: 'bad_request' }, 400);
  try {
    await deleteMailboxAlias(guard.session, id);
    return new Response(null, { status: 204 });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
