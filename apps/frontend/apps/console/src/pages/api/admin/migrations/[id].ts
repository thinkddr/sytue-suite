/*
 * BFF de Sytue Admin — un trabajo de migración.
 *
 *   GET  /api/admin/migrations/:id            Progreso e INFORME (qué se omitió y por qué).
 *   POST /api/admin/migrations/:id?do=cancel  Detiene un trabajo vivo.
 *   POST /api/admin/migrations/:id?do=resume  Lo retoma por su cursor (idempotente).
 *
 * Puro passthrough al gateway con el access token httpOnly. Rol admin/owner.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { loadMigration, migrationAction, proxyStatus } from '../../../../lib/gateway.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

async function guardOf(ctx: Parameters<APIRoute>[0]) {
  return requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['apps.manage'],
  });
}

export const GET: APIRoute = async (ctx) => {
  const guard = await guardOf(ctx);
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);
  try {
    return json(await loadMigration(guard.session, ctx.params.id ?? ''));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};

export const POST: APIRoute = async (ctx) => {
  const guard = await guardOf(ctx);
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);
  const action = new URL(ctx.request.url).searchParams.get('do');
  if (action !== 'cancel' && action !== 'resume') return json({ error: 'not_found' }, 404);
  try {
    return json(await migrationAction(guard.session, ctx.params.id ?? '', action));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
