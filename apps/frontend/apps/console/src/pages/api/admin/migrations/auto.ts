/*
 * BFF de Sytue Admin — lanzar el asistente AUTOMÁTICO de migración.
 *
 *   POST /api/admin/migrations/auto   { instance_id }
 *
 * Encola de una vez todo lo que el plan encontró: un trabajo por persona y tipo de
 * contenido. El plan se vuelve a calcular EN EL GATEWAY, así que lo que se migra no
 * depende de lo que mande el navegador.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { autoMigrate, proxyStatus } from '../../../../lib/gateway.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['apps.manage'],
  });
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);

  const body = (await ctx.request.json().catch(() => null)) as { instance_id?: unknown } | null;
  const instanceId = typeof body?.instance_id === 'string' ? body.instance_id.trim() : '';
  if (!instanceId) return json({ error: 'bad_request' }, 400);
  try {
    return json({ migrations: await autoMigrate(guard.session, instanceId) }, 202);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
