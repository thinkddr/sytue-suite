/*
 * POST /api/services/run { service } -> ejecuta ahora una tarea (cron o job).
 *
 * BFF de ejecución manual. Proxy PURO al control-plane
 * (POST /api/v1/services/{id}/run); sin lógica (invariante). Va por POST, que es
 * además el verbo que exige el checkOrigin CSRF de Astro.
 *
 * Devuelve { ok, live }: live=false cuando no hay backend vivo y la ejecución
 * queda en demostración. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../../lib/auth.js';
import { runManagedService, httpErrorInfo } from '../../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { service?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const service = typeof body.service === 'string' ? body.service.trim() : '';
  if (!service) return json({ error: 'missing_service' }, 400);

  try {
    const live = await runManagedService(session, service);
    return json({ ok: true, live });
  } catch (err) {
    // 409 si el servicio no es una tarea, 404 si aún no está aprovisionada.
    const info = httpErrorInfo(err);
    if (info) return json({ error: info.code }, info.status);
    throw err;
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
