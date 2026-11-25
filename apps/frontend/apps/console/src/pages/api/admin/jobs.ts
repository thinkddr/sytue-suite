/*
 * BFF de Sytue Admin — historial de acciones en lote.
 *
 *   GET /api/admin/jobs -> { jobs[{id, action, total, done, status, created_at, finished_at}] }
 *
 * Proxya a Sytue ID (GET /api/v1/admin/jobs). El lote ya se apuntaba como trabajo
 * —quién, qué, cuántos y cómo acabó cada persona— y nadie podía verlo: cerrada la
 * pestaña, «se aplicó a 28 de 30» no dejaba rastro de cuáles fueron los dos.
 * Exige users.manage, el mismo listón que lanzar el lote.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { loadBulkJobs, proxyStatus } from '../../../lib/admin.js';

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
    perms: ['users.manage'],
  });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  try {
    return json({ jobs: await loadBulkJobs(guard.session) });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
