/*
 * BFF de Sytue Admin — proveedores de servicio SAML de la organización (Sytue ID
 * actuando de IdP para una app de terceros).
 *
 *   GET /api/admin/apps/saml-sps   Lista los SP registrados por la org activa.
 *
 * Proxya a Sytue ID (GET /api/v1/admin/apps/saml-sps, Go) con el access token httpOnly.
 * Rol admin/owner con apps.manage; el aislamiento por organización lo aplica el
 * servicio (un SP de otra org responde 404). El alta la hace hoy `integrations` al
 * activar una plantilla del catálogo: aquí solo se listan, se apagan y se borran.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { loadSamlSps } from '../../../../lib/admin.js';

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
    perms: ['apps.manage'],
  });
  if (guard instanceof Response)
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  const { data } = await loadSamlSps(guard.session);
  return json({ items: data });
};
