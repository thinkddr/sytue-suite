/*
 * BFF de Sytue Admin — PIM self-service (mi acceso privilegiado).
 *
 *   GET  /api/me/pim   Mis elegibilidades y mis activaciones en la org activa.
 *   POST /api/me/pim   body { action: 'activate', role_id?|builtin_role?, justification,
 *                              duration_minutes }  ó  { action: 'deactivate', activation_id }
 *
 * Proxya a Sytue ID (/api/v1/me/pim*, Go) con el access token httpOnly. Cualquier sesión
 * válida basta (la ELEGIBILIDAD, no un rol admin, gobierna qué se puede activar). El token
 * nunca sale al cliente. Sytue ID hace la validación dura (elegibilidad, duración máxima).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { activatePim, deactivatePim, loadMyPim, proxyStatus } from '../../../lib/admin.js';
import type { PimActivateInput } from '../../../lib/types.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** Recorta el cuerpo de activate a role_id XOR builtin_role + justificación + duración. */
function sanitizeActivate(b: Record<string, unknown>): PimActivateInput | null {
  const roleId = typeof b.role_id === 'string' ? b.role_id.trim() : '';
  const builtinRole = typeof b.builtin_role === 'string' ? b.builtin_role.trim() : '';
  const justification = typeof b.justification === 'string' ? b.justification.trim() : '';
  const duration = typeof b.duration_minutes === 'number' ? Math.trunc(b.duration_minutes) : NaN;
  if ((roleId === '') === (builtinRole === '') || !justification || !Number.isFinite(duration)) {
    return null;
  }
  const out: PimActivateInput = { justification, duration_minutes: duration };
  if (roleId) out.role_id = roleId;
  else out.builtin_role = builtinRole;
  return out;
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx));
  if (guard instanceof Response) return json({ error: 'unauthenticated' }, 401);
  const { live, data } = await loadMyPim(guard.session);
  return json({ live, ...data });
};

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx));
  if (guard instanceof Response) return json({ error: 'unauthenticated' }, 401);
  const b = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  const action = b?.action;
  try {
    if (action === 'deactivate') {
      const id = typeof b?.activation_id === 'string' ? b.activation_id.trim() : '';
      if (!id) return json({ error: 'bad_request', message: 'Falta activation_id.' }, 400);
      await deactivatePim(guard.session, id);
      return json({ id, status: 'revoked' });
    }
    if (action === 'activate') {
      const input = sanitizeActivate(b ?? {});
      if (!input) {
        return json(
          { error: 'bad_request', message: 'Indica un rol, la justificación y la duración.' },
          400,
        );
      }
      return json(await activatePim(guard.session, input), 201);
    }
    return json({ error: 'bad_request', message: 'action inválida.' }, 400);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
