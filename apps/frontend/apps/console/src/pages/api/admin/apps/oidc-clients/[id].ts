/*
 * BFF de Sytue Admin — una aplicación OIDC de la organización.
 *
 *   DELETE /api/admin/apps/oidc-clients/:id   La aplicación deja de poder iniciar
 *          sesión al instante (sus códigos y refresh caen en cascada).
 *
 * Proxya a Sytue ID (DELETE /api/v1/admin/apps/oidc-clients/:id, Go). Aislamiento por
 * org en el servicio: la aplicación de otra organización responde 404.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { deleteOidcApp, proxyStatus } from '../../../../../lib/admin.js';

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
    perms: ['apps.manage'],
  });
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);
  const id = ctx.params.id ?? '';
  try {
    await deleteOidcApp(guard.session, id);
    return json({ client_id: id, deleted: true });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
