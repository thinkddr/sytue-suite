/*
 * BFF de Sytue Admin — PIM: elegibilidades (admin).
 *
 *   GET  /api/admin/pim/eligibilities   Lista quién es elegible para qué rol.
 *   POST /api/admin/pim/eligibilities   body { user_id, role_id?|builtin_role?,
 *                                              requires_approval, max_duration_minutes }
 *
 * Proxya a Sytue ID (GET/POST /api/v1/admin/pim/eligibilities, Go) con el access token
 * httpOnly; exige rol admin/owner (gobernar el acceso privilegiado no se delega, como
 * los roles). Propaga el status del servicio (400/403/409). El token nunca sale al cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { createPimEligibility, loadPimEligibilities, proxyStatus } from '../../../../lib/admin.js';
import type { PimEligibilityInput } from '../../../../lib/types.js';

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

/** Recorta el cuerpo a la forma esperada (la validación dura la hace Sytue ID). */
function sanitize(body: unknown): PimEligibilityInput | null {
  const b = (body ?? {}) as Record<string, unknown>;
  const userId = typeof b.user_id === 'string' ? b.user_id.trim() : '';
  const roleId = typeof b.role_id === 'string' ? b.role_id.trim() : '';
  const builtinRole = typeof b.builtin_role === 'string' ? b.builtin_role.trim() : '';
  const maxDuration =
    typeof b.max_duration_minutes === 'number' ? Math.trunc(b.max_duration_minutes) : NaN;
  if (!userId || (roleId === '') === (builtinRole === '') || !Number.isFinite(maxDuration)) {
    return null;
  }
  const out: PimEligibilityInput = {
    user_id: userId,
    requires_approval: Boolean(b.requires_approval),
    max_duration_minutes: maxDuration,
  };
  if (roleId) out.role_id = roleId;
  else out.builtin_role = builtinRole;
  return out;
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const { live, data } = await loadPimEligibilities(guard.session);
  return json({ live, eligibilities: data });
};

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const input = sanitize(await ctx.request.json().catch(() => null));
  if (!input) {
    return json(
      {
        error: 'bad_request',
        message: 'Indica user_id, un rol (role_id o builtin_role) y duración.',
      },
      400,
    );
  }
  try {
    return json(await createPimEligibility(guard.session, input), 201);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
