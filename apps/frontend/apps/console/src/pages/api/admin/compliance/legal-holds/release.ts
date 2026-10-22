/*
 * BFF de Sytue Admin — liberar una retención legal.
 *
 *   POST /api/admin/compliance/legal-holds/release  body { service, id }
 *        Libera el hold {id} en su servicio (mail|drive|notes). Al liberar, los
 *        datos del sujeto vuelven a ser purgables. Acción sensible y AUDITADA en
 *        el servicio.
 *
 * Un hold pertenece a UN servicio, así que la liberación se dirige a ese
 * servicio. Proxya a POST /api/v1/admin/legal-holds/{id}/release (Go) con el
 * token httpOnly; rol admin/owner; propaga el status del servicio.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { releaseHold, proxyStatus, HOLD_SERVICES } from '../../../../../lib/compliance.js';
import type { ComplianceService } from '../../../../../lib/compliance.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);

  const body = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  const service = body?.service;
  const id = typeof body?.id === 'string' ? body.id.trim() : '';
  if (!HOLD_SERVICES.includes(service as ComplianceService) || !id) {
    return json({ error: 'bad_request', message: 'Indica service (mail|drive|notes) e id.' }, 400);
  }
  try {
    await releaseHold(guard.session, service as ComplianceService, id);
    return new Response(null, { status: 204, headers: { 'cache-control': 'no-store' } });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
