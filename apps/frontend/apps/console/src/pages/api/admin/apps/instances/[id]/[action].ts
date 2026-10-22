/*
 * BFF de Sytue Admin — acciones sobre una instancia de integración.
 *
 *   POST /api/admin/apps/instances/:id/activate     Materializa (SP SAML / client OIDC).
 *   POST /api/admin/apps/instances/:id/deactivate   Revierte la materialización.
 *   POST /api/admin/apps/instances/:id/test         Verifica la conexión con el tercero.
 *   POST /api/admin/apps/instances/:id/sync         Encola una reconciliación SCIM.
 *   GET  /api/admin/apps/instances/:id/sync-runs    Historial de sincronizaciones.
 *   PUT   /api/admin/apps/instances/:id/secrets     Escribe el secreto (write-only).
 *   PATCH /api/admin/apps/instances/:id/secrets     Añade uno sin borrar los demás.
 *   GET   /api/admin/apps/instances/:id/readers     Quién puede consultarla (NEG-06).
 *   PUT   /api/admin/apps/instances/:id/readers     Sustituye esa lista.
 *
 * Cada `action` es el segmento LITERAL de la API del gateway: el BFF solo valida el
 * par (método, action) contra una allowlist y reenvía con el access token httpOnly.
 * Rol admin/owner. Puro passthrough (sin lógica). Los secretos no vuelven nunca.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../../server/bff.js';
import { instanceAction, proxyStatus } from '../../../../../../lib/gateway.js';

export const prerender = false;

// action → métodos permitidos. sync-runs y readers se leen; secrets admite PUT
// (reemplaza el conjunto) y PATCH (añade una credencial sin borrar las otras), y la
// lista de quién consulta se sustituye entera.
const ALLOWED: Record<string, ('GET' | 'POST' | 'PUT' | 'PATCH')[]> = {
  activate: ['POST'],
  deactivate: ['POST'],
  test: ['POST'],
  sync: ['POST'],
  'sync-runs': ['GET'],
  secrets: ['PUT', 'PATCH'],
  readers: ['GET', 'PUT'],
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

async function forward(
  ctx: Parameters<APIRoute>[0],
  method: 'GET' | 'POST' | 'PUT' | 'PATCH',
): Promise<Response> {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['apps.manage'],
  });
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);
  const action = ctx.params.action ?? '';
  if (!ALLOWED[action]?.includes(method)) return json({ error: 'not_found' }, 404);
  const body =
    method === 'GET' ? undefined : ((await ctx.request.json().catch(() => ({}))) as unknown);
  try {
    return json(await instanceAction(guard.session, ctx.params.id ?? '', action, method, body));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
}

export const GET: APIRoute = (ctx) => forward(ctx, 'GET');
export const POST: APIRoute = (ctx) => forward(ctx, 'POST');
export const PUT: APIRoute = (ctx) => forward(ctx, 'PUT');
export const PATCH: APIRoute = (ctx) => forward(ctx, 'PATCH');
