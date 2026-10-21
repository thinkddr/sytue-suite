/*
 * POST /api/billing/spend-limit { spend_limit_eur }
 *
 * BFF del límite de gasto mensual del tenant. Proxy PURO a PUT /billing/spend-limit
 * del control-plane (viaja por POST porque el checkOrigin CSRF de Astro rechaza PUT
 * con 403, mismo patrón que /api/alerts). Devuelve { ok, live }: live=false cuando
 * no hay backend vivo y el cambio queda en demostración. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../../lib/auth.js';
import { setSpendLimit, httpErrorInfo } from '../../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { spend_limit_eur?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const limit = Number(body.spend_limit_eur);
  if (!Number.isFinite(limit) || limit < 0) return json({ error: 'invalid_spend_limit' }, 400);

  try {
    const live = await setSpendLimit(session, limit);
    return json({ ok: true, live });
  } catch (err) {
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
