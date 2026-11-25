/*
 * BFF de Sytue Admin — proveedores de servicio SAML de la organización (Sytue ID
 * actuando de IdP para una app de terceros).
 *
 *   GET  /api/admin/apps/saml-sps   Lista los SP registrados por la org activa.
 *   POST /api/admin/apps/saml-sps   body { name, entity_id, acs_url, cert_pem?,
 *        name_id_format?, attribute_mapping?, idp_initiated?, relay_state_default? }
 *        Alta manual de un SP que no está en el catálogo. Responde 201 con el SP y los
 *        datos del IdP que hay que pegar en la app.
 *
 * Proxya a Sytue ID (GET/POST /api/v1/admin/apps/saml-sps, Go) con el access token
 * httpOnly. Rol admin/owner con apps.manage; el aislamiento por organización lo aplica
 * el servicio (un SP de otra org responde 404). Las altas del catálogo las sigue
 * haciendo `integrations` al activar una plantilla.
 *
 * El POST rechaza aquí lo que Sytue ID rechazaría (`lib/saml-sp.ts`), con un `code`
 * por campo, y si Sytue ID dice que no, se propaga su status y su motivo (`proxyError`):
 * un 409 es «ese entity ID ya está registrado» y el admin tiene que saberlo.
 */
import type { APIRoute } from 'astro';
import { requireSession, type GuardOptions } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { createSamlSp, loadSamlSps, proxyError, proxyStatus } from '../../../../lib/admin.js';
import { altaDesdeCuerpo } from '../../../../lib/saml-sp.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

const GUARDA: GuardOptions = { roles: ['admin', 'owner'], perms: ['apps.manage'] };

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), GUARDA);
  if (guard instanceof Response)
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  const { data } = await loadSamlSps(guard.session);
  return json({ items: data });
};

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), GUARDA);
  if (guard instanceof Response)
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  const alta = altaDesdeCuerpo(await ctx.request.json().catch(() => null));
  if (!alta.ok) return json({ error: 'bad_request', code: alta.fallo }, 400);
  try {
    return json(await createSamlSp(guard.session, alta.input), 201);
  } catch (e) {
    return json(proxyError(e), proxyStatus(e));
  }
};
