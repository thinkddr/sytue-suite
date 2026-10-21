/*
 * BFF de Sytue Admin — libertad de elección por producto (Sytue Integration Gateway).
 *
 *   GET  /api/admin/apps/substitutions              Quién sirve cada producto y quién podría.
 *   PUT  /api/admin/apps/substitutions?product=…    body { instance_id }  Elige.
 *        instance_id vacío devuelve el producto al nativo de Sytue.
 *
 * Proxya al gateway (Go) con el access token httpOnly; propaga el status del servicio.
 * Puro passthrough (sin lógica). Leer lo puede cualquier miembro —el mapa es informativo
 * y la RLS ya acota la organización—; ESCRIBIR es admin/owner, porque cambia lo que ve
 * toda la organización al abrir el lanzador, no una preferencia personal.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { loadProductChoices, setProductChoice, proxyStatus } from '../../../../lib/gateway.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx));
  if (guard instanceof Response) return json({ error: 'unauthorized' }, 401);
  const res = await loadProductChoices(guard.session);
  // `live: false` distingue «la pasarela no contesta» de «no hay nada que elegir»: la
  // UI enseña un aviso en el primer caso en vez de un mapa vacío que parece correcto.
  return json({ live: res.live, products: res.data });
};

export const PUT: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['apps.manage'],
  });
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);

  const product = new URL(ctx.request.url).searchParams.get('product')?.trim() ?? '';
  if (!product) return json({ error: 'bad_request', message: 'Falta el producto.' }, 400);

  const body = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  const instanceId = typeof body?.instance_id === 'string' ? body.instance_id.trim() : '';

  try {
    return json(await setProductChoice(guard.session, product, instanceId));
  } catch (e) {
    return json({ error: 'gateway_error' }, proxyStatus(e));
  }
};
