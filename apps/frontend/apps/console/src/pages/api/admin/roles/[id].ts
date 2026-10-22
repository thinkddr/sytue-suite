/*
 * BFF de Sytue Admin — un rol personalizado.
 *
 *   GET    /api/admin/roles/:id                       Detalle + usuarios asignados.
 *   PATCH  /api/admin/roles/:id  body { name?, permissions? }   Edita el rol.
 *   DELETE /api/admin/roles/:id                       Borra el rol.
 *
 * Proxya a Sytue ID (GET/PATCH/DELETE /api/v1/admin/roles/:id, Go) con el access token
 * httpOnly; exige rol admin/owner. Semántica PATCH: solo los campos presentes cambian
 * (permissions presente REEMPLAZA el conjunto). Propaga el status del servicio.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import {
  deleteCustomRole,
  loadRoleDetail,
  proxyStatus,
  updateCustomRole,
} from '../../../../lib/admin.js';
import type { CustomRoleInput } from '../../../../lib/types.js';

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

/** Recorta el PATCH a los campos presentes (ausentes = no tocar). */
function sanitizePatch(body: unknown): CustomRoleInput {
  const b = (body ?? {}) as Record<string, unknown>;
  const out: CustomRoleInput = {};
  if (typeof b.name === 'string') out.name = b.name.trim();
  if (Array.isArray(b.permissions)) {
    out.permissions = b.permissions.map((v) => String(v).trim()).filter(Boolean);
  }
  return out;
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const id = ctx.params.id;
  if (!id) return json({ error: 'bad_request' }, 400);
  try {
    return json(await loadRoleDetail(guard.session, id));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};

export const PATCH: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const id = ctx.params.id;
  if (!id) return json({ error: 'bad_request' }, 400);
  const patch = sanitizePatch(await ctx.request.json().catch(() => null));
  if (patch.name === '') {
    return json({ error: 'bad_request', message: 'El nombre no puede quedar vacío.' }, 400);
  }
  try {
    return json(await updateCustomRole(guard.session, id, patch));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const id = ctx.params.id;
  if (!id) return json({ error: 'bad_request' }, 400);
  try {
    await deleteCustomRole(guard.session, id);
    return json({ id, deleted: true });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
