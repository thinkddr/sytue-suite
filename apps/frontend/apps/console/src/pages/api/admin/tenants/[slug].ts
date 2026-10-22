/*
 * BFF de Sytue Admin — editar ajustes de una organización.
 *
 *   PATCH /api/admin/tenants/{slug}   body { name?, status? }
 *       Edita nombre/estado. Sytue ID exige que el solicitante sea owner/admin de
 *       ESA org (cross-org ⇒ 404); el slug es inmutable.
 *
 * Proxya a Sytue ID (PATCH /api/v1/admin/tenants/{slug}, Go) con el access token
 * httpOnly; propaga el status del servicio (403/404/400/502). Rol admin/owner.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { proxyStatus, updateTenant } from '../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const PATCH: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  const slug = ctx.params.slug ?? '';
  const body = (await ctx.request.json().catch(() => null)) as {
    name?: unknown;
    status?: unknown;
  } | null;
  const patch: { name?: string; status?: 'active' | 'suspended' } = {};
  if (typeof body?.name === 'string') {
    if (!body.name.trim()) {
      return json({ error: 'bad_request', message: 'El nombre no puede quedar vacío.' }, 400);
    }
    patch.name = body.name.trim();
  }
  if (body?.status === 'active' || body?.status === 'suspended') {
    patch.status = body.status;
  }
  if (patch.name === undefined && patch.status === undefined) {
    return json({ error: 'bad_request', message: 'Nada que actualizar.' }, 400);
  }
  try {
    return json(await updateTenant(guard.session, slug, patch));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
