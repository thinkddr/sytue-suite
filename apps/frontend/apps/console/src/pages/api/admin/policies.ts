/*
 * BFF de Sytue Admin — reglas de política de egress (Sytue Integration Gateway).
 *
 *   GET  /api/admin/policies                Lista las reglas de la org activa.
 *   POST /api/admin/policies  body { name, condition, action, enabled }  Crea una regla.
 *
 * Proxya al gateway (GET/POST /api/v1/admin/policies, Go) con el access token
 * httpOnly; propaga el status del servicio. Rol admin/owner.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import {
  createPolicy,
  loadPolicies,
  proxyStatus,
  sanitizePolicyInput,
} from '../../../lib/gateway.js';

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

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const { live, data } = await loadPolicies(guard.session);
  return json({ live, items: data });
};

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const input = sanitizePolicyInput(await ctx.request.json().catch(() => null));
  if (!input) {
    return json(
      { error: 'bad_request', message: 'name y action (allow|warn|block) son obligatorios.' },
      400,
    );
  }
  try {
    return json(await createPolicy(guard.session, input), 201);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
