/*
 * BFF de Sytue Admin — multi-pertenencia: añadir a alguien QUE YA EXISTE a otra
 * organización.
 *
 *   POST /api/admin/tenants/{slug}/members   body { email, role }
 *       Da a una cuenta ya existente una membresía en {slug} con ese rol.
 *
 * Proxya a Sytue ID (POST /api/v1/orgs/{slug}/members, Go) con el access token
 * httpOnly. La AUTORIZACIÓN NO SE DECIDE AQUÍ, y no es un descuido: el servicio
 * exige owner/admin de ESA org concreta comprobándolo contra la BD (el claim de
 * rol del token solo habla de la org activa) y reserva la concesión del rol
 * owner a un owner. Reimplementar ese criterio en el BFF solo podría divergir
 * del de verdad; lo único que se hace aquí es exigir la sesión y propagar.
 *
 * Y se propaga el MOTIVO, no solo el status: los dos 403 de este endpoint son
 * `code: forbidden` y solo el mensaje los distingue («Solo un owner puede
 * asignar el rol owner» frente a «No autorizado en esta organización»).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { addOrgMember, proxyError, proxyStatus } from '../../../../../lib/admin.js';
import type { SytueRole } from '../../../../../lib/types.js';

export const prerender = false;

// Espeja domain.Role de Sytue ID. El servicio revalida: esto solo evita el viaje.
const ROLES: readonly SytueRole[] = ['owner', 'admin', 'member', 'billing'];

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  // Ruta XHR: JSON con el status, nunca la redirección HTML a /auth/login.
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  const slug = ctx.params.slug ?? '';
  const body = (await ctx.request.json().catch(() => null)) as {
    email?: unknown;
    role?: unknown;
  } | null;
  const email = typeof body?.email === 'string' ? body.email.trim() : '';
  const role = body?.role;
  if (!email) {
    return json({ error: 'bad_request', code: 'email_required' }, 400);
  }
  if (typeof role !== 'string' || !ROLES.includes(role as SytueRole)) {
    return json({ error: 'bad_request', code: 'invalid_role' }, 400);
  }
  try {
    return json(await addOrgMember(guard.session, slug, { email, role: role as SytueRole }));
  } catch (e) {
    return json(proxyError(e), proxyStatus(e));
  }
};
