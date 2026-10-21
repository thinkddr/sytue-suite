/*
 * BFF de Sytue Admin — disparar el reseteo de contraseña por email.
 *
 *   POST /api/admin/users/:id/reset-password   -> { id, sent }
 *       Envía al usuario un enlace de reseteo para que ELIJA su nueva contraseña.
 *       El admin no la ve ni la fija (alternativa a .../password). Rol admin/owner.
 *
 * Proxya a Sytue ID (POST /api/v1/admin/users/:id/reset-password, Go) con el
 * access token httpOnly; propaga el status del servicio (403/404/409). El token
 * nunca sale al cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { sendPasswordResetLink, proxyStatus } from '../../../../../lib/admin.js';

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
    return json(await sendPasswordResetLink(guard.session, id));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
