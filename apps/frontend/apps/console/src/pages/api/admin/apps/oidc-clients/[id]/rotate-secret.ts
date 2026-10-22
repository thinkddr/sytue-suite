/*
 * BFF de Sytue Admin — rotación del secreto de una aplicación OIDC.
 *
 *   POST /api/admin/apps/oidc-clients/:id/rotate-secret   Devuelve el secreto NUEVO
 *        una sola vez; el anterior deja de valer al instante.
 *
 * Proxya a Sytue ID (POST /api/v1/admin/apps/oidc-clients/:id/rotate-secret, Go). El
 * secreto solo viaja en esta respuesta: no se guarda en la Console ni se vuelve a leer.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../../server/bff.js';
import { proxyStatus, rotateOidcAppSecret } from '../../../../../../lib/admin.js';

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
    perms: ['apps.manage'],
  });
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);
  try {
    return json(await rotateOidcAppSecret(guard.session, ctx.params.id ?? ''));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
