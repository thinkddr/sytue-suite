/*
 * BFF de Sytue Admin — regla de política concreta (Sytue Integration Gateway).
 *
 *   PATCH  /api/admin/policies/:id  body { name, condition, action, enabled }
 *          Reemplaza la regla (estado deseado completo, incl. toggle enabled).
 *   DELETE /api/admin/policies/:id  Borra la regla.
 *
 * Proxya al gateway (PATCH/DELETE /api/v1/admin/policies/:id, Go); rol admin/owner.
 * Tras el cambio, el gateway invalida la caché del motor de política.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import {
  deletePolicy,
  proxyStatus,
  sanitizePolicyInput,
  updatePolicy,
} from '../../../../lib/gateway.js';

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

export const PATCH: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const id = ctx.params.id ?? '';
  const input = sanitizePolicyInput(await ctx.request.json().catch(() => null));
  if (!id || !input) {
    return json(
      { error: 'bad_request', message: 'name y action (allow|warn|block) son obligatorios.' },
      400,
    );
  }
  try {
    return json(await updatePolicy(guard.session, id, input));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const id = ctx.params.id ?? '';
  if (!id) return json({ error: 'bad_request' }, 400);
  try {
    await deletePolicy(guard.session, id);
    return json({ ok: true });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
