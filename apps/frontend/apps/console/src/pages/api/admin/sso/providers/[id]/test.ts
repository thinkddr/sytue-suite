/*
 * BFF de Sytue Admin — probar el descubrimiento de un proveedor SSO.
 *
 *   POST /api/admin/sso/providers/:id/test   Re-ejecuta el discovery del issuer
 *        (SSRF-safe) y actualiza los endpoints cacheados; devuelve el proveedor.
 *
 * Proxya a Sytue ID (POST /api/v1/admin/sso/providers/:id/test, Go) con el access
 * token httpOnly; exige rol admin/owner de ESA org (cross-org ⇒ 404). Propaga el
 * status del servicio (400 si el issuer no resuelve).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../../server/bff.js';
import { proxyStatus, testSsoProvider } from '../../../../../../lib/admin.js';

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
    perms: ['sso.manage'],
  });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  const id = ctx.params.id ?? '';
  try {
    return json(await testSsoProvider(guard.session, id));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
