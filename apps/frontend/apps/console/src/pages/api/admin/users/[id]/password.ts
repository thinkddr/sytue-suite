/*
 * BFF de Sytue Admin — reset de contraseña de un usuario.
 *
 *   POST /api/admin/users/:id/password   body { new_password: string }
 *       Fija una contraseña al usuario y revoca sus sesiones. Rol admin/owner.
 *
 * Proxya a Sytue ID (POST /api/v1/admin/users/:id/password, Go) con el access
 * token httpOnly; propaga el status del servicio (403/404/400). La contraseña
 * nunca sale al cliente (solo servidor→servidor) ni se registra.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { resetUserPassword, proxyStatus } from '../../../../../lib/admin.js';

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

  const body = (await ctx.request.json().catch(() => null)) as { new_password?: unknown } | null;
  const pw = body?.new_password;
  if (typeof pw !== 'string' || pw.length < 8) {
    return json(
      { error: 'bad_request', message: 'La contraseña debe tener al menos 8 caracteres.' },
      400,
    );
  }
  try {
    return json(await resetUserPassword(guard.session, id, pw));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
