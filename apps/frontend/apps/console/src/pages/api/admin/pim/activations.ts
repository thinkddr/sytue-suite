/*
 * BFF de Sytue Admin — PIM: activaciones (admin).
 *
 *   GET  /api/admin/pim/activations?status=pending|active   Solicitudes / vigentes.
 *   POST /api/admin/pim/activations   body { activation_id, action: 'approve'|'deny' }
 *
 * Proxya a Sytue ID (/api/v1/admin/pim/activations*, Go) con el access token httpOnly;
 * exige rol admin/owner. approve/deny actúan sobre una activación PENDIENTE concreta.
 * Propaga el status del servicio (403/404). El token nunca sale al cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import {
  approvePimActivation,
  denyPimActivation,
  loadPimActivations,
  proxyStatus,
} from '../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

function guardStatus(s: number): Response {
  return json({ error: s === 403 ? 'forbidden' : 'unauthenticated' }, s === 403 ? 403 : 401);
}

const STATUSES = new Set(['pending', 'active', 'revoked', 'denied', 'expired']);

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const status = ctx.url.searchParams.get('status') ?? '';
  const { live, data } = await loadPimActivations(
    guard.session,
    STATUSES.has(status) ? status : undefined,
  );
  return json({ live, activations: data });
};

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const b = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  const id = typeof b?.activation_id === 'string' ? b.activation_id.trim() : '';
  const action = b?.action;
  if (!id || (action !== 'approve' && action !== 'deny')) {
    return json({ error: 'bad_request', message: 'Falta activation_id o action inválida.' }, 400);
  }
  try {
    if (action === 'approve') return json(await approvePimActivation(guard.session, id));
    await denyPimActivation(guard.session, id);
    return json({ id, status: 'denied' });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
