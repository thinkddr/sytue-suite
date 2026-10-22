/*
 * BFF de Sytue Admin — registro de egress (Sytue Integration Gateway).
 *
 *   GET /api/admin/egress-audit[?host=&status=&since=&until=&cursor=&limit=]
 *       Página del registro de egress de la org activa. { live, items, nextCursor }.
 *
 * Proxya al gateway (GET /api/v1/admin/egress-audit, Go) con el access token
 * httpOnly; exige rol admin/owner. El gateway valida filtros/cursor y acota por
 * tenant (RLS). Sin backend vivo degrada a vacío (live=false).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { loadEgressPage } from '../../../lib/gateway.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  const p = new URL(ctx.request.url).searchParams;
  const limit = Number.parseInt(p.get('limit') ?? '', 10);
  const { live, data } = await loadEgressPage(guard.session, {
    host: p.get('host')?.trim() || undefined,
    status: p.get('status')?.trim() || undefined,
    since: p.get('since')?.trim() || undefined,
    until: p.get('until')?.trim() || undefined,
    cursor: p.get('cursor')?.trim() || undefined,
    ...(Number.isFinite(limit) && limit > 0 ? { limit } : {}),
  });
  return json({ live, items: data.items, nextCursor: data.nextCursor });
};
