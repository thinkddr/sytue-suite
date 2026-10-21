/*
 * BFF de Sytue Admin — mis organizaciones (selector multi-org, self-service).
 *
 *   GET /api/me/organizations   Mis organizaciones activas, con cuál lo está.
 *   PUT /api/me/organizations   body { org_id }  → cambia mi organización activa.
 *
 * Proxya a Sytue ID (GET /api/v1/me y PUT /api/v1/me/active-organization, Go) con el
 * access token httpOnly. Cualquier sesión válida basta: la verja es la MEMBRESÍA, no un
 * rol admin, y la aplica Sytue ID. El token nunca sale al cliente.
 *
 * Después de un PUT con éxito el token del navegador sigue llevando el `tenant` anterior
 * —va firmado dentro—, así que el cliente tiene que rehacer el login SSO. La respuesta lo
 * dice con `reauthorize: true` en vez de dejarlo a que cada pantalla se acuerde.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { loadMyOrganizations, proxyStatus, setMyActiveOrganization } from '../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx));
  if (guard instanceof Response) return json({ error: 'unauthenticated' }, 401);
  const { live, data } = await loadMyOrganizations(guard.session);
  return json({ live, organizations: data });
};

export const PUT: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx));
  if (guard instanceof Response) return json({ error: 'unauthenticated' }, 401);
  const b = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  const orgId = typeof b?.org_id === 'string' ? b.org_id.trim() : '';
  if (!orgId) return json({ error: 'bad_request', message: 'Falta org_id.' }, 400);
  try {
    const { slug } = await setMyActiveOrganization(guard.session, orgId);
    return json({ slug, reauthorize: true });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
