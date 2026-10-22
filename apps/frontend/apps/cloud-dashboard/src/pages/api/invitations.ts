/*
 * BFF de invitaciones de equipo.
 *   POST { email, role }          -> invita o reenvía
 *   POST { id, delete: true }     -> revoca
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { inviteMember, revokeMemberInvitation, httpErrorInfo } from '../../lib/cloud.js';

export const prerender = false;

const ROLES = new Set(['owner', 'admin', 'member', 'billing']);

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { email?: unknown; role?: unknown; id?: unknown; delete?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  try {
    if (body.delete === true) {
      const id = typeof body.id === 'string' ? body.id.trim() : '';
      if (!id) return json({ error: 'missing_id' }, 400);
      await revokeMemberInvitation(session, id);
      return json({ ok: true, live: true });
    }

    const email = typeof body.email === 'string' ? body.email.trim() : '';
    const role = typeof body.role === 'string' ? body.role.trim() : '';
    if (!email) return json({ error: 'missing_email' }, 400);
    if (!ROLES.has(role)) return json({ error: 'invalid_role' }, 400);
    const result = await inviteMember(session, email, role);
    return json({ ok: true, live: true, ...result }, 201);
  } catch (err) {
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
