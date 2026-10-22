/*
 * POST /api/services/delete { service } -> da de baja un servicio gestionado.
 *
 * BFF de baja. Proxy PURO al control-plane (DELETE /api/v1/services/{id}); sin
 * lógica (invariante). Va por POST y no por DELETE porque el checkOrigin CSRF de
 * Astro rechaza DELETE con 403 (mismo patrón que /api/env y /api/alerts).
 *
 * Hasta ahora NO existía: se podían crear servicios desde la web y no retirarlos,
 * así que una base de datos o una función creadas por error se quedaban ahí —y
 * facturando— salvo que alguien tirara de curl. La confirmación de una acción
 * destructiva es cosa de la interfaz; la autorización real (admin/owner) la impone
 * el control-plane. Devuelve { ok, live }: live=false cuando no hay backend vivo y
 * la baja queda en demostración. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../../lib/auth.js';
import { deleteManagedService, httpErrorInfo } from '../../../lib/cloud.js';

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
    const live = await deleteManagedService(session, service);
    return json({ ok: true, live });
  } catch (err) {
    // Error REAL del control-plane (403 sin rol, 404 ya retirado…): status real.
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
