/*
 * BFF de Sytue Admin — Roles personalizados / admin delegada.
 *
 *   GET  /api/admin/roles   Lista los roles de la org activa (con nº de asignados).
 *   POST /api/admin/roles   body { name, permissions[] }   Crea un rol.
 *
 * Proxya a Sytue ID (GET/POST /api/v1/admin/roles, Go) con el access token httpOnly;
 * exige rol admin/owner (definir roles NO se delega: anti auto-escalada). Propaga el
 * status del servicio (400/403/409). El token nunca sale al cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { createCustomRole, loadCustomRoles, proxyStatus } from '../../../lib/admin.js';

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

/** Recorta el cuerpo a { name, permissions[] } (la validación dura la hace Sytue ID). */
function sanitize(body: unknown): { name: string; permissions: string[] } {
  const b = (body ?? {}) as Record<string, unknown>;
  const name = typeof b.name === 'string' ? b.name.trim() : '';
  const permissions = Array.isArray(b.permissions)
    ? b.permissions.map((v) => String(v).trim()).filter(Boolean)
    : [];
  return { name, permissions };
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const { live, data } = await loadCustomRoles(guard.session);
  return json({ live, roles: data });
};

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const input = sanitize(await ctx.request.json().catch(() => null));
  if (!input.name) {
    return json({ error: 'bad_request', message: 'El nombre del rol es obligatorio.' }, 400);
  }
  try {
    return json(await createCustomRole(guard.session, input), 201);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
