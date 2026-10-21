/*
 * BFF de Sytue Admin — plan del asistente AUTOMÁTICO de migración.
 *
 *   GET /api/admin/migrations/plan?instance_id=…
 *
 * Devuelve qué se traería con la conexión ya autorizada (quién se conectó, qué
 * tiene, en qué cuentas de Sytue aterriza) SIN copiar nada. Proxya al Sytue
 * Integration Gateway, que es quien pregunta al origen y al directorio.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { loadMigrationPlan, proxyStatus } from '../../../../lib/gateway.js';

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
    perms: ['apps.manage'],
  });
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);

  const instanceId = (ctx.url.searchParams.get('instance_id') ?? '').trim();
  if (!instanceId) return json({ error: 'bad_request' }, 400);
  try {
    return json({ plan: await loadMigrationPlan(guard.session, instanceId) });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
