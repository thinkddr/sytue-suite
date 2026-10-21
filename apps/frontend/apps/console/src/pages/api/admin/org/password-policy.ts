/*
 * BFF de Sytue Admin — política de contraseñas de la organización.
 *
 *   GET   /api/admin/org/password-policy  -> { min_length, require_complexity }
 *   PATCH /api/admin/org/password-policy  body { min_length?, require_complexity? }
 *       Lee/edita la política de la org activa. min_length acotado 8..128.
 *       Rol admin/owner.
 *
 * Proxya a Sytue ID (/api/v1/admin/org/password-policy, Go) con el access token
 * httpOnly; propaga el status del servicio (403/400). El token nunca sale al
 * cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { loadPasswordPolicy, updatePasswordPolicy, proxyStatus } from '../../../../lib/admin.js';

export const prerender = false;

const MIN = 8;
const MAX = 128;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

async function guardAdmin(ctx: Parameters<APIRoute>[0]) {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  return guard;
}

export const GET: APIRoute = async (ctx) => {
  const guard = await guardAdmin(ctx);
  if (guard instanceof Response) return guard;
  const { data } = await loadPasswordPolicy(guard.session);
  return json(data);
};

export const PATCH: APIRoute = async (ctx) => {
  const guard = await guardAdmin(ctx);
  if (guard instanceof Response) return guard;

  const body = (await ctx.request.json().catch(() => null)) as {
    min_length?: unknown;
    require_complexity?: unknown;
  } | null;
  const patch: { min_length?: number; require_complexity?: boolean } = {};
  if (body?.min_length !== undefined) {
    const n = Number(body.min_length);
    if (!Number.isInteger(n) || n < MIN || n > MAX) {
      return json(
        { error: 'bad_request', message: `La longitud mínima debe estar entre ${MIN} y ${MAX}.` },
        400,
      );
    }
    patch.min_length = n;
  }
  if (typeof body?.require_complexity === 'boolean') {
    patch.require_complexity = body.require_complexity;
  }
  if (patch.min_length === undefined && patch.require_complexity === undefined) {
    return json({ error: 'bad_request', message: 'Nada que actualizar.' }, 400);
  }
  try {
    return json(await updatePasswordPolicy(guard.session, patch));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
