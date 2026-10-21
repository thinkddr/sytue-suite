/*
 * BFF de Sytue Admin — decisiones de política (Sytue Integration Gateway).
 *
 *   GET /api/admin/policy-decisions[?action=warn|block&cursor=&limit=]
 *       Página de decisiones (block/warn) de la org activa. { live, items, nextCursor }.
 *
 * Proxya al gateway (GET /api/v1/admin/policy-decisions, Go); rol admin/owner.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { loadDecisionsPage } from '../../../lib/gateway.js';

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
  const { live, data } = await loadDecisionsPage(guard.session, {
    action: p.get('action')?.trim() || undefined,
    cursor: p.get('cursor')?.trim() || undefined,
    ...(Number.isFinite(limit) && limit > 0 ? { limit } : {}),
  });
  return json({ live, items: data.items, nextCursor: data.nextCursor });
};
