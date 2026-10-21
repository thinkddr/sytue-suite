/*
 * GET /api/services/logs?service=<id>&lines=<n> -> registro de un servicio gestionado
 *
 * BFF de logs de servicios gestionados (función, tarea programada, trabajo, app
 * del catálogo). Proxy PURO al control-plane (GET /api/v1/services/{id}/logs);
 * sin lógica (invariante).
 *
 * Los logs de la APP del proyecto ya se veían por otro camino; los de sus
 * servicios no existían en ninguna pantalla, así que una función que no arranca
 * era un callejón sin salida. La autorización real (member+, solo humanos) la
 * impone el control-plane. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../../lib/auth.js';
import { loadServiceLogs } from '../../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const params = new URL(ctx.request.url).searchParams;
  const service = params.get('service')?.trim();
  if (!service) return json({ error: 'missing_service' }, 400);
  // El tope real lo impone el control-plane; aquí solo se evita mandarle basura.
  const lines = Math.min(Math.max(Number(params.get('lines')) || 200, 1), 1000);

  const { data, live } = await loadServiceLogs(session, service, lines);
  return json({ lines: data, live });
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
