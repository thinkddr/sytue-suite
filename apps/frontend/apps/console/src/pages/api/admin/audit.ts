/*
 * BFF de Sytue Admin — log de auditoría (paginación keyset + filtros).
 *
 *   GET /api/admin/audit[?action=&actor=&since=&until=&cursor=&limit=]
 *       Página del log de la organización activa. { live, items, nextCursor }.
 *
 * Proxya al endpoint admin de Sytue ID (GET /api/v1/admin/audit, Go) con el
 * access token httpOnly de la sesión; exige rol admin/owner. El servicio Go
 * valida los filtros y el cursor (opaco) y acota el ámbito al tenant del token.
 * Sin backend vivo degrada al seed (live=false, sin cursor).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { loadAudit } from '../../../lib/admin.js';

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
    perms: ['audit.view'],
  });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }

  const p = new URL(ctx.request.url).searchParams;
  const limit = Number.parseInt(p.get('limit') ?? '', 10);
  const { live, data } = await loadAudit(guard.session, {
    action: p.get('action')?.trim() || undefined,
    actor: p.get('actor')?.trim() || undefined,
    since: p.get('since')?.trim() || undefined,
    until: p.get('until')?.trim() || undefined,
    cursor: p.get('cursor')?.trim() || undefined,
    ...(Number.isFinite(limit) && limit > 0 ? { limit } : {}),
  });
  return json({ live, items: data.items, nextCursor: data.nextCursor });
};
