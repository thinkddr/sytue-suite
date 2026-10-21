/*
 * BFF de Sytue Admin — editar/borrar una política de Acceso Condicional.
 *
 *   PATCH  /api/admin/access-policies/:id   body { …campos a tocar }   Edita la política.
 *   DELETE /api/admin/access-policies/:id    Borra la política.
 *
 * Proxya a Sytue ID (PATCH/DELETE /api/v1/admin/access-policies/:id, Go) con el access
 * token httpOnly; exige rol admin/owner de ESA org (cross-org ⇒ 404). Semántica PATCH:
 * solo los campos presentes se modifican.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import {
  deleteAccessPolicy,
  proxyStatus,
  sanitizeAccessPolicyInput,
  updateAccessPolicy,
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

export const PATCH: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['access_policies.manage'],
  });
  if (guard instanceof Response) return guardStatus(guard.status);
  const id = ctx.params.id ?? '';
  const patch = sanitizeAccessPolicyInput(await ctx.request.json().catch(() => null));
  try {
    return json(await updateAccessPolicy(guard.session, id, patch));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['access_policies.manage'],
  });
  if (guard instanceof Response) return guardStatus(guard.status);
  const id = ctx.params.id ?? '';
  try {
    await deleteAccessPolicy(guard.session, id);
    return json({ id, deleted: true });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
