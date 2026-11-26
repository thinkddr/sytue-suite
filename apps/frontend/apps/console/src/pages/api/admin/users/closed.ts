/*
 * BFF de Sytue Admin — cuentas dadas de baja que aún se pueden restaurar.
 *
 *   GET /api/admin/users/closed -> { users[{id, email, name, closed_at, closed_by?,
 *                                   reason?, restorable_until}], retention_days }
 *
 * Proxya a Sytue ID (GET /api/v1/admin/users/closed). Sin este listado la
 * restauración era inalcanzable desde la consola: el directorio no enseña las
 * cuentas cerradas, y restaurar exigía saberse un uuid de memoria. Lectura, con
 * el listón del directorio (users.read o users.manage).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { loadClosedUsers, proxyStatus } from '../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['users.read', 'users.manage'],
  });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  try {
    return json(await loadClosedUsers(guard.session));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
