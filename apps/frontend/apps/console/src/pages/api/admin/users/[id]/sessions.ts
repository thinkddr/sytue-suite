/*
 * BFF de Sytue Admin — sesiones (dispositivos) de un usuario.
 *
 *   GET    /api/admin/users/:id/sessions   -> { sessions: [...] }
 *   DELETE /api/admin/users/:id/sessions   -> { id, sessions_revoked }  (cierre forzado)
 *
 * Proxya a Sytue ID (GET .../sessions y POST .../sessions/revoke, Go) con el
 * access token httpOnly; exige rol admin/owner. Propaga el status del servicio.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { loadUserSessions, revokeUserSessions, proxyStatus } from '../../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

function unauthorized(guard: Response): Response {
  return json(
    { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
    guard.status === 403 ? 403 : 401,
  );
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['users.read', 'users.manage'],
  });
  if (guard instanceof Response) return unauthorized(guard);
  const id = ctx.params.id;
  if (!id) return json({ error: 'bad_request' }, 400);
  try {
    return json({ sessions: await loadUserSessions(guard.session, id) });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['users.read', 'users.manage'],
  });
  if (guard instanceof Response) return unauthorized(guard);
  const id = ctx.params.id;
  if (!id) return json({ error: 'bad_request' }, 400);
  try {
    return json(await revokeUserSessions(guard.session, id));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
