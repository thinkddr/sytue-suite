/*
 * BFF de miembros de la organización.
 *   POST { sub, role }          -> añade o cambia el rol
 *   POST { sub, delete: true }  -> quita
 *
 * Los miembros determinan los PUESTOS DE SUITE facturables, así que estas
 * mutaciones tienen efecto económico: nunca se simulan sin control-plane.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { addMember, removeMember, httpErrorInfo } from '../../lib/cloud.js';

export const prerender = false;

const ROLES = new Set(['owner', 'admin', 'member', 'billing']);

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { sub?: unknown; role?: unknown; delete?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const sub = typeof body.sub === 'string' ? body.sub.trim() : '';
  if (!sub) return json({ error: 'missing_sub' }, 400);

  try {
    if (body.delete === true) {
      await removeMember(session, sub);
      return json({ ok: true, live: true });
    }
    const role = typeof body.role === 'string' ? body.role.trim() : '';
    if (!ROLES.has(role)) return json({ error: 'invalid_role' }, 400);
    const member = await addMember(session, sub, role);
    return json({ ok: true, live: true, member });
  } catch (err) {
    // El control-plane rechaza dejar la org sin owner: ese 409 debe llegar tal cual.
    const info = httpErrorInfo(err);
    if (info) return json({ error: info.code }, info.status);
    return json({ error: 'control_plane_unavailable' }, 503);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
