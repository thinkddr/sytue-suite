/*
 * BFF de Sytue Admin — Acceso Condicional (paridad Entra Conditional Access).
 *
 *   GET  /api/admin/access-policies   Lista las políticas de la org activa.
 *   POST /api/admin/access-policies   body { name, action, enabled?, priority?,
 *        include_users?, include_groups?, exclude_users?, exclude_groups?, ip_ranges?,
 *        ip_negate?, client_ids?, days_mask?, start_minute?, end_minute? }   Crea una.
 *
 * Proxya a Sytue ID (GET/POST /api/v1/admin/access-policies, Go) con el access token
 * httpOnly; propaga el status del servicio (400/403/404). Rol admin/owner. El
 * enforcement de las políticas vive en el LOGIN del servicio, no aquí.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import {
  createAccessPolicy,
  loadAccessPolicies,
  proxyStatus,
  sanitizeAccessPolicyInput,
} from '../../../lib/admin.js';
import type { AccessPolicyInput } from '../../../lib/types.js';

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
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['access_policies.manage'],
  });
  if (guard instanceof Response) return guardStatus(guard.status);
  const { live, data } = await loadAccessPolicies(guard.session);
  return json({ live, items: data });
};

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['access_policies.manage'],
  });
  if (guard instanceof Response) return guardStatus(guard.status);
  const input = sanitizeAccessPolicyInput(await ctx.request.json().catch(() => null));
  if (!input.name || !input.action) {
    return json(
      {
        error: 'bad_request',
        message: 'name y action (allow|require_mfa|block) son obligatorios.',
      },
      400,
    );
  }
  try {
    return json(await createAccessPolicy(guard.session, input as AccessPolicyInput), 201);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
